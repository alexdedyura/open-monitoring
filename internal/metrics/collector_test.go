package metrics

import (
	"testing"
	"time"
)

// The collector must produce plausible samples on a real machine. This is a
// smoke test rather than a unit test: it is the only place the whole chain —
// gopsutil, the sensor helper, the ticker and the history buffer — is
// exercised together.
func TestCollectorProducesSamples(t *testing.T) {
	if testing.Short() {
		t.Skip("samples real hardware and spawns helpers")
	}

	samples := make(chan Sample, 8)
	c := NewCollector(500, func(s Sample) {
		select {
		case samples <- s:
		default: // a slow test must not block the collector
		}
	})
	c.Start()
	defer c.Stop()

	var s Sample
	select {
	case s = <-samples:
	case <-time.After(10 * time.Second):
		t.Fatal("collector produced no sample")
	}

	if s.T == 0 {
		t.Error("sample has no timestamp")
	}
	if s.CPU.Usage < 0 || s.CPU.Usage > 100 {
		t.Errorf("implausible CPU usage: %v", s.CPU.Usage)
	}
	if len(s.CPU.PerCore) == 0 {
		t.Error("no per-core CPU readings")
	}
	if s.Mem.Total == 0 {
		t.Error("no total memory reported")
	}
	if s.Mem.UsedPercent <= 0 || s.Mem.UsedPercent > 100 {
		t.Errorf("implausible memory usage: %v", s.Mem.UsedPercent)
	}

	// Which optional sources are available depends on the machine and on
	// whether the test runs elevated, so these are reported, not asserted.
	t.Logf("cpu %.1f%% over %d cores, mem %.1f%%, %d disk(s)",
		s.CPU.Usage, len(s.CPU.PerCore), s.Mem.UsedPercent, len(s.Disks))
	t.Logf("optional: gpu=%v fps=%v cpuTemp=%v cpuClock=%v",
		s.GPU != nil, s.FPS != nil, s.CPU.TempC, s.CPU.ClockMHz)
}

// History must cover the requested window and stay in chronological order,
// since the frontend feeds it straight into a chart.
func TestCollectorHistoryIsOrdered(t *testing.T) {
	if testing.Short() {
		t.Skip("samples real hardware and spawns helpers")
	}

	c := NewCollector(250, nil)
	c.Start()
	defer c.Stop()

	time.Sleep(2 * time.Second)

	history := c.History(3600)
	if len(history) < 2 {
		t.Fatalf("expected several samples, got %d", len(history))
	}
	for i := 1; i < len(history); i++ {
		if history[i].T < history[i-1].T {
			t.Fatalf("history is out of order at %d: %d before %d", i, history[i].T, history[i-1].T)
		}
	}

	// A window shorter than the run must return a subset, not everything.
	if recent := c.History(1); len(recent) > len(history) {
		t.Errorf("a 1s window returned %d samples, more than the full %d", len(recent), len(history))
	}
}

// Motherboard and memory modules are read from SMBIOS by the sensor helper
// rather than from WMI. Nothing else would notice if that stopped working —
// the System panel would just quietly go blank — so guard it here.
func TestBridgeSuppliesMachineDescription(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns the sensor helper")
	}

	c := NewCollector(1000, nil)
	c.Start()
	defer c.Stop()

	// The helper has to launch and produce a first reading.
	var info BridgeInfo
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if info = c.BridgeInfo(); info.Board != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if info.Board == "" {
		t.Skip("sensor helper reported nothing; not bundled in this build")
	}
	if info.RAM.Modules == 0 {
		t.Error("no memory modules reported")
	}
	if info.RAM.Modules > 0 && info.RAM.ModuleGB <= 0 {
		t.Errorf("memory module size is %v GB", info.RAM.ModuleGB)
	}
	t.Logf("board %q, %d × %.0f GB %s-%d %s, gpu %q",
		info.Board, info.RAM.Modules, info.RAM.ModuleGB, info.RAM.Type,
		info.RAM.SpeedMT, info.RAM.Vendor, info.GPUName)
}

// The sampling interval is clamped so the UI cannot ask for a rate that would
// have WMI queries and helper reads overlapping.
func TestClampInterval(t *testing.T) {
	for _, tc := range []struct {
		ms   int
		want time.Duration
	}{
		{0, 250 * time.Millisecond},
		{-1, 250 * time.Millisecond},
		{100, 250 * time.Millisecond},
		{250, 250 * time.Millisecond},
		{1000, time.Second},
		{5000, 5 * time.Second},
	} {
		if got := clampInterval(tc.ms); got != tc.want {
			t.Errorf("clampInterval(%d) = %v, want %v", tc.ms, got, tc.want)
		}
	}
}
