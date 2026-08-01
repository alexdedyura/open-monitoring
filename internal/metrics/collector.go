// Package metrics samples the machine and hands the app a stream of Samples.
//
// Four sources feed a Sample, each covering what the others cannot:
//
//   - gopsutil (system.go)   — CPU load, memory, disk and network throughput.
//     Pure Go, no privileges needed.
//   - lhm-bridge (sensors.go) — GPU telemetry for every vendor, CPU package
//     temperature and power. A bundled helper process, because these live
//     behind vendor APIs and a kernel driver.
//   - PresentMon (fps_*.go)  — frame times of the foreground application.
//   - WMI (sysinfo_*.go, smart_*.go, cpuclock_*.go) — static machine
//     description, drive identity, and a driver-free CPU clock.
//
// Collector owns their lifecycle and merges their output on a fixed interval.
//
// Beside them it owns a ProcessSource (process_*.go): a table of every running
// process, refreshed on a two-second clock of its own and served on request. It
// never enters a Sample — a per-process table is a view of the machine right
// now rather than a series, and enumerating it on the sampling interval would
// put it on the goroutine that also drives alerts and recording.
package metrics

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// minIntervalMs bounds how fast the app may sample. Below this the WMI and
// helper reads start overlapping and cost more than the data is worth.
const minIntervalMs = 250

// historyCap is how many samples are kept for hydrating charts on startup —
// one hour at the default one-second interval.
const historyCap = 3600

// fpsInterval is how often the frame-rate readout and the HUD's FPS and
// frame-time graphs are refreshed. It is deliberately independent of the
// sampling interval: a full sample reads WMI and the sensor helper and cannot
// run this often, while FPS is computed from frame times already buffered in
// memory. At one sample per second the readout was a second behind the game.
const fpsInterval = 100 * time.Millisecond

// Collector samples every source on a fixed interval, keeps a bounded history
// for chart hydration, and notifies a subscriber on each new sample.
type Collector struct {
	system   *systemSampler
	sensors  *SensorSource
	fps      *FPSSource
	cpuClock *CPUClockSource
	procs    *ProcessSource

	onSample func(Sample)
	onFPS    func(*FPSMetrics)

	mu      sync.Mutex
	history []Sample

	interval  time.Duration
	intervalC chan time.Duration
	cancel    context.CancelFunc

	diskMu     sync.Mutex
	diskHealth []DiskHealthView
}

// NewCollector wires up the sources. Optional ones that are unavailable on
// this machine start as nil and are simply skipped when sampling.
//
// onFPS, if set, is called on its own faster cadence with frame-rate metrics
// only — see fpsInterval.
func NewCollector(intervalMs int, onSample func(Sample), onFPS func(*FPSMetrics)) *Collector {
	return &Collector{
		system:    newSystemSampler(),
		sensors:   StartSensors(),
		fps:       StartFPS(),
		cpuClock:  StartCPUClock(),
		procs:     StartProcesses(),
		onSample:  onSample,
		onFPS:     onFPS,
		interval:  clampInterval(intervalMs),
		intervalC: make(chan time.Duration, 1),
	}
}

func (c *Collector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.refreshDiskHealth() // prime the cache without delaying startup
	go c.run(ctx)
	if c.onFPS != nil && c.fps != nil {
		go c.runFPS(ctx)
	}
}

func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.sensors != nil {
		c.sensors.Stop()
	}
	if c.fps != nil {
		c.fps.Stop()
	}
	if c.cpuClock != nil {
		c.cpuClock.Stop()
	}
	if c.procs != nil {
		c.procs.Stop()
	}
}

// SetInterval changes the sampling period on the fly. A full channel means a
// change is already queued, and the newest value is the one that matters.
func (c *Collector) SetInterval(ms int) {
	select {
	case c.intervalC <- clampInterval(ms):
	default:
	}
}

func clampInterval(ms int) time.Duration {
	if ms < minIntervalMs {
		ms = minIntervalMs
	}
	return time.Duration(ms) * time.Millisecond
}

func (c *Collector) run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case d := <-c.intervalC:
			c.interval = d
			ticker.Reset(d)

		case <-ticker.C:
			s := c.collect()

			c.mu.Lock()
			c.history = append(c.history, s)
			if len(c.history) > historyCap {
				c.history = c.history[len(c.history)-historyCap:]
			}
			c.mu.Unlock()

			if c.onSample != nil {
				c.onSample(s)
			}
		}
	}
}

// runFPS pushes frame-rate metrics between samples. Nothing is emitted while
// no application is presenting, except the one update that clears a readout
// that has just gone stale — an idle desktop should not wake the frontend ten
// times a second.
func (c *Collector) runFPS(ctx context.Context) {
	ticker := time.NewTicker(fpsInterval)
	defer ticker.Stop()

	had := false
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			m := c.fps.Metrics()
			if m == nil && !had {
				continue
			}
			had = m != nil
			c.onFPS(m)
		}
	}
}

// collect merges one reading from every source into a Sample.
func (c *Collector) collect() Sample {
	now := time.Now()
	s := Sample{T: now.UnixMilli()}

	c.system.sample(&s, now)

	if c.sensors != nil {
		if r := c.sensors.Latest(); r != nil {
			s.CPU.TempC = r.CPU.TempC
			s.CPU.PowerW = r.CPU.PowerW
			s.CPU.ClockMHz = r.CPU.ClockMHz
			if r.GPU != nil {
				gpu := r.GPU.GPUMetrics
				s.GPU = &gpu
			}
		}
	}

	// The helper only reports a clock when PawnIO is installed; performance
	// counters give the same number without a driver.
	if s.CPU.ClockMHz == 0 && c.cpuClock != nil {
		s.CPU.ClockMHz = c.cpuClock.MHz()
	}

	if c.fps != nil {
		s.FPS = c.fps.Metrics()
	}

	if c.system.spaceDue(now) {
		go c.system.refreshSpace()
		go c.refreshDiskHealth()
	}
	return s
}

// History returns the buffered samples covering the last `seconds` seconds.
func (c *Collector) History(seconds int) []Sample {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(seconds) * time.Second).UnixMilli()
	i := sort.Search(len(c.history), func(i int) bool { return c.history[i].T >= cutoff })

	out := make([]Sample, len(c.history)-i)
	copy(out, c.history[i:])
	return out
}

// DiskHealth returns the cached drive list. The WMI queries behind it take
// seconds on some machines, so they never run on a caller's goroutine — the UI
// would appear frozen while one was in flight.
func (c *Collector) DiskHealth() []DiskHealthView {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	out := make([]DiskHealthView, len(c.diskHealth))
	copy(out, c.diskHealth)
	return out
}

// Processes returns the most recent process table. Enumeration runs on the
// source's own clock (see processInterval), so this is a cache read — the
// Processes tab polls it, and polling faster only re-reads the same table.
func (c *Collector) Processes() ProcessSnapshot {
	if c.procs == nil {
		return ProcessSnapshot{}
	}
	return c.procs.Snapshot()
}

// refreshDiskHealth re-reads drive identity and SMART counters from WMI.
func (c *Collector) refreshDiskHealth() {
	disks := physicalDisks()

	c.diskMu.Lock()
	c.diskHealth = disks
	c.diskMu.Unlock()
}

// Static gathers the one-time machine description shown in the system panel,
// plus the current state of each optional source.
func (c *Collector) Static() StaticInfo {
	info := StaticInfo{OS: runtime.GOOS}

	if h, err := host.Info(); err == nil {
		info.OS = h.Platform + " " + h.PlatformVersion
		info.Hostname = h.Hostname
		info.BootTime = int64(h.BootTime)
	}
	if ci, err := cpu.Info(); err == nil && len(ci) > 0 {
		info.CPUModel = ci[0].ModelName
		info.CPUBaseMHz = ci[0].Mhz
	}
	if n, err := cpu.Counts(false); err == nil {
		info.CPUCores = n // physical
	}
	if n, err := cpu.Counts(true); err == nil {
		info.CPUThreads = n
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		info.RAMTotal = vm.Total
	}
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			info.Disks = append(info.Disks, trimSlash(p.Device))
		}
	}

	info.IsAdmin = isElevated()
	info.OSScale = osScale()
	info.OSLang = osLang()

	info.SourceStatus = c.Status()
	info.ApplyBridgeInfo(c.BridgeInfo())
	return info
}

// BridgeInfo returns the parts of the machine description that only the sensor
// helper knows. They stay empty until it has produced its first reading, which
// is why they are refreshed rather than captured once with the rest of Static.
func (c *Collector) BridgeInfo() BridgeInfo {
	var out BridgeInfo
	if c.sensors == nil {
		return out
	}
	r := c.sensors.Latest()
	if r == nil {
		return out
	}

	if r.GPU != nil {
		out.GPUName = r.GPU.Name
	}
	if r.System != nil {
		out.Board = r.System.Board
		if r.System.Memory != nil {
			out.RAM = *r.System.Memory
		}
	}
	return out
}

// Status reports which optional sources are currently delivering data. Unlike
// Static it is cheap, because the UI polls it: a helper may still have been
// starting up when the window first asked.
func (c *Collector) Status() SourceStatus {
	st := SourceStatus{}
	if c.fps != nil {
		st.FPSOK = c.fps.Running()
		st.FPSError = c.fps.LastError()
	}

	if c.sensors == nil {
		return st
	}
	st.SensorsError = c.sensors.LastError()
	if r := c.sensors.Latest(); r != nil {
		st.SensorsOK = true
		st.PawnIO = r.PawnIO
		st.PawnIOVersion = r.PawnIOVersion
	}
	return st
}

// ResetFPS restarts the frame-rate average and lows. See FPSSource.Reset.
func (c *Collector) ResetFPS() {
	if c.fps != nil {
		c.fps.Reset()
	}
}

// RestartSensors relaunches the sensor helper. Needed after PawnIO is
// installed: LibreHardwareMonitor decides once, at type initialisation,
// whether the driver is present, so a running helper never notices it appear.
func (c *Collector) RestartSensors() {
	if c.sensors != nil {
		c.sensors.Restart()
	}
}
