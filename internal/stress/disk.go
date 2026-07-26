package stress

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// A drive has two very different limits and a stress test has to hit both:
// streaming bandwidth, which is what a big sequential transfer measures, and
// command throughput, which only shows up under small random requests with
// several of them in flight at once. NVMe drives in particular look idle under
// sequential load — the controller only gets warm, and SLC caches only run
// out, once thousands of small writes are queued against it.
//
// The file is opened unbuffered where the OS allows it (see disk_windows.go),
// because otherwise this measures the page cache: a 2 GB "disk test" on a
// machine with 32 GB of RAM would never reach the platters or the flash.
const (
	diskSeqBlock   = 4 << 20 // sequential transfer size
	diskRandBlock  = 4 << 10 // random request size — the classic IOPS unit
	diskQueueDepth = 16      // requests in flight during the random phases
	diskRandPhase  = 12 * time.Second
	diskFileName   = "open-monitoring-stress.tmp"
)

func runDisk(ctx context.Context, o Options, rep *reporter) error {
	dir := diskDir(o.DiskPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot use %s: %w", dir, err)
	}

	size := int64(o.DiskMB) << 20
	if usage, err := disk.Usage(dir); err == nil {
		// Never take more than half of what is free: filling a system drive
		// is a far worse outcome than a slightly less punishing test.
		if half := int64(usage.Free / 2); size > half {
			size = half / diskSeqBlock * diskSeqBlock
		}
	}
	if size < 64<<20 {
		return fmt.Errorf("not enough free space in %s for a disk test", dir)
	}

	path := filepath.Join(dir, diskFileName)
	f, direct, err := openDirect(path)
	if err != nil {
		return fmt.Errorf("cannot create the test file in %s: %w", dir, err)
	}
	// The file is removed even when the run is cancelled or the app shuts
	// down mid-test — a leftover multi-gigabyte temp file would be rude.
	defer os.Remove(path)
	defer f.Close()

	mode := "buffered (cache bypass unavailable)"
	if direct {
		mode = "unbuffered · cache and write-back bypassed"
	}
	rep.setDetail("%s · %.1f GB file · %s · QD%d random",
		dir, float64(size)/(1<<30), mode, diskQueueDepth)

	rep.set("Sequential write", 0, "MB/s")
	rep.set("Sequential read", 0, "MB/s")
	rep.set("Random write", 0, "IOPS")
	rep.set("Random read", 0, "IOPS")
	rep.set("Mixed 70/30 read", 0, "IOPS")
	rep.set("Data written", 0, "GB")

	var written atomic.Int64
	best := map[string]float64{}
	record := func(label string, v float64, unit string) {
		best[label] = math.Max(best[label], v)
		rep.set(label, best[label], unit)
	}

	// Written bytes are the one number a user may want to stop the test over,
	// so it is refreshed continuously rather than at the end of a round.
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				rep.set("Data written", float64(written.Load())/(1<<30), "GB")
			}
		}
	}()

	for round := int64(1); ; round++ {
		rep.setPhase("round %d · sequential write", round)
		if mbs, ok := diskSequential(ctx, f, size, true, &written); ok {
			record("Sequential write", mbs, "MB/s")
		} else {
			return nil
		}

		rep.setPhase("round %d · sequential read", round)
		if mbs, ok := diskSequential(ctx, f, size, false, &written); ok {
			record("Sequential read", mbs, "MB/s")
		} else {
			return nil
		}

		phases := []struct {
			label      string
			writeRatio float64
		}{
			{"Random write", 1.0},
			{"Random read", 0.0},
			{"Mixed 70/30 read", 0.3},
		}
		for _, p := range phases {
			rep.setPhase("round %d · %s, QD%d", round, strings.ToLower(p.label), diskQueueDepth)
			// The phase reports as it goes: at 12 s each, waiting for the end
			// would leave the panel showing zeroes for most of a short run.
			progress := func(iops float64) { record(p.label, iops, "IOPS") }
			if _, ok := diskRandom(ctx, f, size, p.writeRatio, &written, progress); !ok {
				return nil
			}
		}
	}
}

// diskDir normalises what the UI hands over. The drive picker sends "C:",
// which Windows resolves to the process's current directory on that drive
// rather than its root — not what anyone means by "test drive C".
func diskDir(p string) string {
	if p == "" {
		return os.TempDir()
	}
	if len(p) == 2 && p[1] == ':' {
		return p + string(os.PathSeparator)
	}
	return p
}

// diskSequential streams the whole file in one direction and returns MB/s.
func diskSequential(ctx context.Context, f *os.File, size int64, write bool, written *atomic.Int64) (float64, bool) {
	buf := alignedBuf(diskSeqBlock)
	if write {
		fillIncompressible(buf, uint64(time.Now().UnixNano()))
	}

	start := time.Now()
	for off := int64(0); off+diskSeqBlock <= size; off += diskSeqBlock {
		if ctx.Err() != nil {
			return 0, false
		}
		var err error
		if write {
			_, err = f.WriteAt(buf, off)
			written.Add(diskSeqBlock)
		} else {
			_, err = f.ReadAt(buf, off)
		}
		if err != nil {
			return 0, ctx.Err() == nil
		}
	}
	if write {
		f.Sync() // no-op when unbuffered, essential when not
	}

	secs := time.Since(start).Seconds()
	if secs <= 0 {
		return 0, true
	}
	return float64(size) / secs / (1 << 20), true
}

// diskRandom issues small requests at a fixed queue depth for a fixed time,
// calling progress with the rate so far about once a second, and returns the
// final IOPS. writeRatio 0 is read-only, 1 is write-only.
func diskRandom(parent context.Context, f *os.File, size int64, writeRatio float64, written *atomic.Int64, progress func(float64)) (float64, bool) {
	ctx, cancel := context.WithTimeout(parent, diskRandPhase)
	defer cancel()

	slots := size / diskRandBlock
	var ops atomic.Int64

	start := time.Now()
	var wg sync.WaitGroup
	for q := range diskQueueDepth {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			buf := alignedBuf(diskRandBlock)
			fillIncompressible(buf, uint64(seed))

			for ctx.Err() == nil {
				off := rng.Int63n(slots) * diskRandBlock
				var err error
				if rng.Float64() < writeRatio {
					_, err = f.WriteAt(buf, off)
					written.Add(diskRandBlock)
				} else {
					_, err = f.ReadAt(buf, off)
				}
				if err != nil {
					return
				}
				ops.Add(1)
			}
		}(time.Now().UnixNano() + int64(q))
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for waiting := true; waiting; {
		select {
		case <-done:
			waiting = false
		case now := <-tick.C:
			if secs := now.Sub(start).Seconds(); secs > 0 && progress != nil {
				progress(float64(ops.Load()) / secs)
			}
		}
	}

	// The phase's own deadline expiring is normal — the run only ends when
	// the job's context does.
	if parent.Err() != nil {
		return 0, false
	}
	secs := time.Since(start).Seconds()
	if secs <= 0 {
		return 0, true
	}
	return float64(ops.Load()) / secs, true
}

// fillIncompressible defeats the transparent compression some controllers
// apply: a buffer of zeroes would be written at the speed of the DRAM cache
// and report a bandwidth the drive cannot sustain with real data.
func fillIncompressible(buf []byte, seed uint64) {
	x := seed | 1
	for i := 0; i+8 <= len(buf); i += 8 {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		for b := range 8 {
			buf[i+b] = byte(x >> (8 * b))
		}
	}
}
