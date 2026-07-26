package stress

import (
	"math"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestClampRejectsNonsense(t *testing.T) {
	o := Options{
		Targets:    []string{"cpu", "cpu", "gpu", "toaster"},
		Seconds:    -5,
		CPUThreads: 1 << 20,
		RAMPercent: 300,
		GPUPercent: 0,
		DiskMB:     1,
	}
	Clamp(&o)

	if got := len(o.Targets); got != 2 {
		t.Fatalf("targets = %v, want the two known ones de-duplicated", o.Targets)
	}
	if o.Seconds != 0 {
		t.Errorf("Seconds = %d, want 0 (run until stopped)", o.Seconds)
	}
	if o.CPUThreads != runtime.NumCPU() {
		t.Errorf("CPUThreads = %d, want the default %d", o.CPUThreads, runtime.NumCPU())
	}
	if o.RAMPercent != 70 || o.GPUPercent != 80 || o.DiskMB != 2048 {
		t.Errorf("out-of-range tunables not reset: %+v", o)
	}
	if o.DiskPath == "" {
		t.Error("DiskPath should fall back to the temp directory")
	}
}

// The vector kernels are hand-written assembly, so the only thing standing
// between a mis-encoded instruction and a wrong GFLOPS figure is this: every
// width has to compute the same converging chain the portable loop does.
func TestVectorKernelsAgree(t *testing.T) {
	seed := [4]float64{1.0, 0.9999997, 1e-7, 0}
	const iters = 10_000

	want := fmaGeneric(iters, &seed) / fmaAccums
	for _, k := range []struct {
		name string
		fn   func(int64, *[4]float64) float64
	}{
		{"selected", vectorKernel(true).fn},
		{"selected-no-avx512", vectorKernel(false).fn},
	} {
		got := k.fn(iters, &seed) / fmaAccums
		// Each lane runs the same chain, so the mean per accumulator is the
		// same value regardless of how many lanes there are.
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s kernel = %v, want %v", k.name, got, want)
		}
	}
}

func TestVectorKernelIsNotOptimisedAway(t *testing.T) {
	seed := [4]float64{1.0, 0.9999997, 1e-7, 0}
	vec := vectorKernel(true)

	start := time.Now()
	vec.fn(5_000_000, &seed)
	if elapsed := time.Since(start); elapsed < time.Millisecond {
		t.Errorf("%s ran 5M rounds in %v — the loop is not doing the work", vec.name, elapsed)
	}
}

func TestRAMPatternIsAddressDependent(t *testing.T) {
	const seed = 42
	if ramPattern(0, seed) == ramPattern(1, seed) {
		t.Error("neighbouring words share a pattern; coupling faults would go unseen")
	}
	if ramPattern(1000, seed) == ramPattern(1000, seed+1) {
		t.Error("the pattern ignores the seed, so every pass would write the same bits")
	}
}

// A short real run: the CPU job must produce numbers, stop when its deadline
// passes, and leave the runner idle.
func TestRunnerCPUJobCompletes(t *testing.T) {
	if testing.Short() {
		t.Skip("burns every core for two seconds")
	}

	var last Status
	r := NewRunner(func(s Status) { last = s })

	o := Defaults()
	o.Targets = []string{CPU}
	o.Seconds = 2
	o.CPUThreads = 2
	if _, err := r.Start(o); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && r.Status().Running {
		time.Sleep(100 * time.Millisecond)
	}

	st := r.Status()
	if st.Running {
		t.Fatal("the CPU job outlived its deadline")
	}
	if len(st.Jobs) != 1 || st.Jobs[0].State != StateDone {
		t.Fatalf("final status = %+v, want one finished job", st.Jobs)
	}
	for _, s := range st.Jobs[0].Stats {
		if s.Label == "Peak vector" && s.Value <= 0 {
			t.Error("no vector throughput was measured")
		}
	}
	_ = last
}

func TestDiskDirNormalisesDriveLetters(t *testing.T) {
	if got := diskDir("C:"); got != "C:"+string(os.PathSeparator) {
		t.Errorf("diskDir(%q) = %q, want the drive root", "C:", got)
	}
	if diskDir("") == "" {
		t.Error("an empty path should fall back to the temp directory")
	}
}

func TestStopEndsAnOpenRun(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a real CPU job")
	}

	r := NewRunner(nil)
	o := Defaults()
	o.Targets = []string{CPU}
	o.Seconds = 0 // until stopped
	o.CPUThreads = 1
	if _, err := r.Start(o); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !r.Status().Running {
		t.Fatal("the job did not start")
	}

	r.StopAll()
	if st := r.Status(); st.Running {
		t.Fatal("StopAll returned with the job still running")
	}
}
