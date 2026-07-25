package metrics

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

// systemSampler reads everything gopsutil can provide without help from a
// native sensor helper: CPU load, memory, and disk / network throughput.
//
// Disk and network counters are monotonic totals, so a rate needs the previous
// snapshot and the time between the two. That state lives here.
type systemSampler struct {
	prevIO    map[string]disk.IOCountersStat
	prevIOAt  time.Time
	prevNet   gnet.IOCountersStat
	prevNetAt time.Time

	mu      sync.Mutex
	usage   map[string]float64 // drive -> used space %
	usageAt time.Time
}

// spaceRefresh is how often free-space is re-scanned. Unlike throughput it
// barely moves, and scanning every mounted volume is slow enough that doing it
// on the sampling interval would show up as a stutter.
const spaceRefresh = 30 * time.Second

func newSystemSampler() *systemSampler {
	cpu.Percent(0, true) // prime the delta-based reading; the first call is meaningless
	return &systemSampler{usage: map[string]float64{}}
}

// sample fills in the parts of s that come from gopsutil. Sources that fail
// leave their fields untouched, which reads as "unavailable" downstream.
func (ss *systemSampler) sample(s *Sample, now time.Time) {
	ss.sampleCPU(s)
	ss.sampleMem(s)
	ss.sampleDisks(s, now)
	ss.sampleNet(s, now)
}

func (ss *systemSampler) sampleCPU(s *Sample) {
	perCore, err := cpu.Percent(0, true)
	if err != nil || len(perCore) == 0 {
		return
	}

	s.CPU.PerCore = make([]float64, len(perCore))
	total := 0.0
	for i, v := range perCore {
		s.CPU.PerCore[i] = round1(v)
		total += v
	}
	s.CPU.Usage = round1(total / float64(len(perCore)))
}

func (ss *systemSampler) sampleMem(s *Sample) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return
	}
	s.Mem = MemMetrics{Total: vm.Total, Used: vm.Used, UsedPercent: round1(vm.UsedPercent)}

	// Page file: on Windows gopsutil reads size from the commit limit and usage
	// from the "Paging File % Usage" counter. A failure just leaves the fields
	// at zero, which the UI renders as an empty chart.
	if sw, err := mem.SwapMemory(); err == nil {
		s.Mem.SwapTotal, s.Mem.SwapUsed = sw.Total, sw.Used
	}
}

func (ss *systemSampler) sampleDisks(s *Sample, now time.Time) {
	io, err := disk.IOCounters()
	if err != nil || len(io) == 0 {
		return
	}

	elapsed := now.Sub(ss.prevIOAt).Seconds()

	ss.mu.Lock()
	usage := ss.usage // replaced wholesale by refreshSpace, so the map itself is safe to hold
	ss.mu.Unlock()

	for name, cur := range io {
		d := DiskMetrics{Name: name, UsedPercent: usage[name]}
		if prev, ok := ss.prevIO[name]; ok && elapsed > 0 {
			d.ReadBps = math.Max(0, float64(cur.ReadBytes-prev.ReadBytes)/elapsed)
			d.WriteBps = math.Max(0, float64(cur.WriteBytes-prev.WriteBytes)/elapsed)
		}
		s.Disks = append(s.Disks, d)
	}
	sort.Slice(s.Disks, func(i, j int) bool { return s.Disks[i].Name < s.Disks[j].Name })

	ss.prevIO, ss.prevIOAt = io, now
}

func (ss *systemSampler) sampleNet(s *Sample, now time.Time) {
	counters, err := gnet.IOCounters(false) // false = totals across all interfaces
	if err != nil || len(counters) == 0 {
		return
	}

	cur := counters[0]
	elapsed := now.Sub(ss.prevNetAt).Seconds()
	if !ss.prevNetAt.IsZero() && elapsed > 0 {
		s.Net.UpBps = math.Max(0, float64(cur.BytesSent-ss.prevNet.BytesSent)/elapsed)
		s.Net.DownBps = math.Max(0, float64(cur.BytesRecv-ss.prevNet.BytesRecv)/elapsed)
	}
	ss.prevNet, ss.prevNetAt = cur, now
}

// spaceDue reports whether free-space is stale enough to re-scan, and claims
// the refresh so only one caller starts it.
func (ss *systemSampler) spaceDue(now time.Time) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if now.Sub(ss.usageAt) < spaceRefresh {
		return false
	}
	ss.usageAt = now
	return true
}

// refreshSpace re-scans used space per volume. Slow enough to be worth running
// off the sampling goroutine.
func (ss *systemSampler) refreshSpace() {
	parts, err := disk.Partitions(false) // false = physical volumes only
	if err != nil {
		return
	}

	fresh := make(map[string]float64, len(parts))
	for _, p := range parts {
		u, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}
		key := p.Device
		if key == "" {
			key = p.Mountpoint
		}
		fresh[trimSlash(key)] = round1(u.UsedPercent)
	}

	ss.mu.Lock()
	ss.usage = fresh
	ss.mu.Unlock()
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// trimSlash normalises volume keys: gopsutil reports "C:" from IOCounters but
// "C:\\" from Partitions, and the two have to line up.
func trimSlash(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\\' || s[len(s)-1] == '/') {
		s = s[:len(s)-1]
	}
	return s
}
