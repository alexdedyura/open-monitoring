package metrics

import (
	"context"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

// Collector samples all metric sources on a fixed interval, keeps an in-memory
// ring buffer for chart hydration and notifies a subscriber on every sample.
type Collector struct {
	mu       sync.Mutex
	ring     []Sample
	ringCap  int
	interval time.Duration
	onSample func(Sample)

	nvidia *NvidiaSource
	lhm    *LHMSource

	prevIO    map[string]disk.IOCountersStat
	prevIOAt  time.Time
	prevNet   gnet.IOCountersStat
	prevNetAt time.Time

	usage     map[string]float64 // drive -> used space %
	usageAt   time.Time
	cancel    context.CancelFunc
	intervalC chan time.Duration
}

func NewCollector(intervalMs int, lhmURL string, onSample func(Sample)) *Collector {
	c := &Collector{
		ringCap:   3600, // up to 1h of hydration history at 1s
		interval:  time.Duration(intervalMs) * time.Millisecond,
		onSample:  onSample,
		usage:     map[string]float64{},
		intervalC: make(chan time.Duration, 1),
	}
	c.nvidia = StartNvidia()
	c.lhm = StartLHM(lhmURL)
	return c
}

func (c *Collector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.run(ctx)
}

func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.nvidia != nil {
		c.nvidia.Stop()
	}
	if c.lhm != nil {
		c.lhm.Stop()
	}
}

// SetInterval changes the sampling period on the fly.
func (c *Collector) SetInterval(ms int) {
	if ms < 250 {
		ms = 250
	}
	select {
	case c.intervalC <- time.Duration(ms) * time.Millisecond:
	default:
	}
}

func (c *Collector) run(ctx context.Context) {
	cpu.Percent(0, true) // prime the delta-based reading
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-c.intervalC:
			c.interval = d
			t.Reset(d)
		case <-t.C:
			s := c.collect()
			c.mu.Lock()
			c.ring = append(c.ring, s)
			if len(c.ring) > c.ringCap {
				c.ring = c.ring[len(c.ring)-c.ringCap:]
			}
			c.mu.Unlock()
			if c.onSample != nil {
				c.onSample(s)
			}
		}
	}
}

// History returns the buffered samples covering the last `seconds` seconds.
func (c *Collector) History(seconds int) []Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(seconds)*time.Second).UnixMilli()
	i := sort.Search(len(c.ring), func(i int) bool { return c.ring[i].T >= cutoff })
	out := make([]Sample, len(c.ring)-i)
	copy(out, c.ring[i:])
	return out
}

func (c *Collector) collect() Sample {
	now := time.Now()
	s := Sample{T: now.UnixMilli()}

	if pc, err := cpu.Percent(0, true); err == nil && len(pc) > 0 {
		s.CPU.PerCore = make([]float64, len(pc))
		sum := 0.0
		for i, v := range pc {
			s.CPU.PerCore[i] = round1(v)
			sum += v
		}
		s.CPU.Usage = round1(sum / float64(len(pc)))
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.Mem = MemMetrics{Total: vm.Total, Used: vm.Used, UsedPercent: round1(vm.UsedPercent)}
	}

	if c.nvidia != nil {
		s.GPU = c.nvidia.Latest()
	}

	if c.lhm != nil {
		if r := c.lhm.Latest(); r != nil {
			s.CPU.TempC = r.CPUTemp
			s.CPU.PowerW = r.CPUPower
			s.CPU.ClockMHz = r.CPUClock
			if s.GPU == nil && r.GPU != nil {
				s.GPU = r.GPU // AMD/Intel GPUs surface through LibreHardwareMonitor
			}
		}
	}

	c.collectDisks(&s, now)
	c.collectNet(&s, now)
	return s
}

func (c *Collector) collectDisks(s *Sample, now time.Time) {
	io, err := disk.IOCounters()
	if err == nil && len(io) > 0 {
		dt := now.Sub(c.prevIOAt).Seconds()
		for name, cur := range io {
			d := DiskMetrics{Name: name}
			if prev, ok := c.prevIO[name]; ok && dt > 0 {
				d.ReadBps = math.Max(0, float64(cur.ReadBytes-prev.ReadBytes)/dt)
				d.WriteBps = math.Max(0, float64(cur.WriteBytes-prev.WriteBytes)/dt)
			}
			d.UsedPercent = c.usage[name]
			s.Disks = append(s.Disks, d)
		}
		c.prevIO = io
		c.prevIOAt = now
		sort.Slice(s.Disks, func(i, j int) bool { return s.Disks[i].Name < s.Disks[j].Name })
	}

	if now.Sub(c.usageAt) > 30*time.Second {
		c.usageAt = now
		go c.refreshUsage()
	}
}

func (c *Collector) refreshUsage() {
	parts, err := disk.Partitions(false)
	if err != nil {
		return
	}
	fresh := map[string]float64{}
	for _, p := range parts {
		if u, err := disk.Usage(p.Mountpoint); err == nil {
			key := p.Device
			if key == "" {
				key = p.Mountpoint
			}
			fresh[trimSlash(key)] = round1(u.UsedPercent)
		}
	}
	c.mu.Lock()
	c.usage = fresh
	c.mu.Unlock()
}

func (c *Collector) collectNet(s *Sample, now time.Time) {
	nio, err := gnet.IOCounters(false)
	if err != nil || len(nio) == 0 {
		return
	}
	cur := nio[0]
	dt := now.Sub(c.prevNetAt).Seconds()
	if !c.prevNetAt.IsZero() && dt > 0 {
		s.Net.UpBps = math.Max(0, float64(cur.BytesSent-c.prevNet.BytesSent)/dt)
		s.Net.DownBps = math.Max(0, float64(cur.BytesRecv-c.prevNet.BytesRecv)/dt)
	}
	c.prevNet = cur
	c.prevNetAt = now
}

// Static gathers one-time machine info.
func (c *Collector) Static() StaticInfo {
	info := StaticInfo{OS: runtime.GOOS}
	if h, err := host.Info(); err == nil {
		info.OS = h.Platform + " " + h.PlatformVersion
	}
	if ci, err := cpu.Info(); err == nil && len(ci) > 0 {
		info.CPUModel = ci[0].ModelName
		info.CPUCores = int(ci[0].Cores)
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
	if c.nvidia != nil {
		info.GPUName = c.nvidia.Name()
		info.NvidiaSMI = true
	}
	if c.lhm != nil {
		if r := c.lhm.Latest(); r != nil {
			info.LHMConnected = true
			if info.GPUName == "" {
				info.GPUName = r.GPUName
			}
		}
	}
	return info
}

// LHMAlive reports whether LibreHardwareMonitor's endpoint currently responds.
func (c *Collector) LHMAlive() bool {
	return c.lhm != nil && c.lhm.Latest() != nil
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func trimSlash(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\\' || s[len(s)-1] == '/') {
		s = s[:len(s)-1]
	}
	return s
}
