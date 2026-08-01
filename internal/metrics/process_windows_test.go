//go:build windows

package metrics

import (
	"math"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The CPU column is the number users will hold up against Task Manager, so the
// convention it encodes — a share of the whole machine, not of one core — is
// worth pinning down.
func TestCPUPercent(t *testing.T) {
	const oneSecond = uint64(1e7) // 100-ns ticks

	for _, tc := range []struct {
		name    string
		delta   uint64
		elapsed float64
		cpus    int
		want    float64
	}{
		{"a whole single-core second", oneSecond, 1, 1, 100},
		{"one core of eight", oneSecond, 1, 8, 12.5},
		{"half a core of four", oneSecond / 2, 1, 4, 12.5},
		{"idle", 0, 1, 8, 0},
		{"measured over two seconds", 2 * oneSecond, 2, 1, 100},
		{"no elapsed time to divide by", oneSecond, 0, 8, 0},
		{"no core count to divide by", oneSecond, 1, 0, 0},
		{"more time than the clock allows", 1000 * oneSecond, 1, 8, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cpuPercent(tc.delta, tc.elapsed, tc.cpus); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("cpuPercent(%d, %v, %d) = %v, want %v", tc.delta, tc.elapsed, tc.cpus, got, tc.want)
			}
		})
	}
}

func TestRatePerSec(t *testing.T) {
	if got := ratePerSec(3000, 1000, 2); got != 1000 {
		t.Errorf("2000 bytes over 2 s = %v B/s, want 1000", got)
	}
	if got := ratePerSec(1000, 1000, 2); got != 0 {
		t.Errorf("an unchanged counter = %v B/s, want 0", got)
	}
	if got := ratePerSec(3000, 1000, 0); got != 0 {
		t.Errorf("no elapsed time = %v B/s, want 0", got)
	}
}

// These counters only ever climb, so one that went backwards belongs to a
// different process under a recycled PID. The subtraction is unsigned, and
// eighteen quintillion bytes per second is not a plausible reading.
func TestDeltaIgnoresACounterThatWentBackwards(t *testing.T) {
	if got := delta(1000, 3000); got != 0 {
		t.Errorf("delta(1000, 3000) = %d, want 0", got)
	}
	if got := ratePerSec(1000, 3000, 2); got != 0 {
		t.Errorf("backwards counter = %v B/s, want 0", got)
	}
}

// The name of a process that has one, and a fallback for the kernel-owned
// entries that do not.
func TestProcessNameFallsBackToThePID(t *testing.T) {
	name, err := windows.NewNTUnicodeString("chrome.exe")
	if err != nil {
		t.Fatalf("building a test name: %v", err)
	}
	if got := processName(1234, *name); got != "chrome.exe" {
		t.Errorf("named pid 1234 = %q, want %q", got, "chrome.exe")
	}

	if got := processName(4, windows.NTUnicodeString{}); got != "System" {
		t.Errorf("pid 4 with no image name = %q, want %q", got, "System")
	}
	if got := processName(1234, windows.NTUnicodeString{}); got != "pid 1234" {
		t.Errorf("nameless pid 1234 = %q, want %q", got, "pid 1234")
	}
}

// The whole chain against the live machine: one enumeration, two of them a
// delta apart, and the rates that come out of the pair. The logged timing is
// the measurement behind processInterval — if a refresh ever costs more than a
// few milliseconds here, that constant is the thing to raise.
func TestProcessSourceEnumerates(t *testing.T) {
	if testing.Short() {
		t.Skip("enumerates the live process table")
	}

	p := StartProcesses()
	defer p.Stop()

	// Two refreshes have to land before any row can carry a rate.
	time.Sleep(2*processInterval + 500*time.Millisecond)

	snap := p.Snapshot()
	if snap.Error != "" {
		t.Fatalf("enumeration failed: %s", snap.Error)
	}
	if snap.At == 0 {
		t.Fatal("no enumeration completed")
	}
	if len(snap.Rows) == 0 {
		t.Fatal("no processes reported")
	}
	if snap.Threads <= 0 {
		t.Errorf("total threads = %d, want a positive count", snap.Threads)
	}

	self := os.Getpid()
	var me *ProcessRow
	for i := range snap.Rows {
		r := &snap.Rows[i]
		if r.PID == 0 {
			t.Error("the idle process is in the table; its CPU time is the machine's idle time")
		}
		if r.CPU < 0 || r.CPU > 100 {
			t.Errorf("pid %d (%s) reports %v%% CPU", r.PID, r.Name, r.CPU)
		}
		if r.PID == self {
			me = r
		}
	}
	if me == nil {
		t.Fatalf("the test's own pid %d is not in the table", self)
	}
	if me.Threads == 0 {
		t.Error("the test process reports no threads")
	}
	if me.MemBytes == 0 {
		t.Error("the test process reports no working set")
	}

	// The cost of one refresh, measured on a source of its own rather than on
	// p, whose loop goroutine owns prev/buf and would be racing this call.
	solo := &ProcessSource{cpus: p.cpus, prev: map[procKey]procPrev{}, buf: make([]byte, processBufSize)}
	start := time.Now()
	solo.refresh(start)
	t.Logf("refresh of %d processes took %v", len(snap.Rows), time.Since(start))
	t.Logf("self: %s pid %d, %.1f%% cpu, %d MB, %d threads",
		me.Name, me.PID, me.CPU, me.MemBytes>>20, me.Threads)
}
