package stress

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// Memory has three properties worth stressing, and they need three different
// loops: capacity (claim it and keep it resident), bandwidth (stream through
// it faster than the controller likes), and integrity (write a known pattern
// and read it back — the only part of this app that can find an actual fault
// rather than just a temperature).
//
// One pass covers all three: write the pattern, verify it, invert every word
// in place — a read-and-write sweep over the whole region, which is where the
// bandwidth number comes from — then verify the inverse. Every sweep moves
// real data and every sweep is checked, so the run reports both a throughput
// figure and an error count.
const (
	ramBlockBytes  = 128 << 20 // one allocation unit
	ramReserveFree = 1 << 30   // never claim the last gigabyte of available memory
)

func runRAM(ctx context.Context, o Options, rep *reporter) error {
	budget, err := ramBudget(o.RAMPercent)
	if err != nil {
		return err
	}

	rep.setDetail("%.1f GB target · %d streaming threads · pattern verify each pass",
		float64(budget)/(1<<30), runtime.NumCPU())
	rep.set("Allocated", 0, "GB")
	rep.set("Bandwidth", 0, "GB/s")
	rep.set("Peak bandwidth", 0, "GB/s")
	rep.set("Random latency", 0, "ns")
	rep.set("Passes", 0, "")

	// Claim the memory in blocks, reporting as it fills. Touching every word
	// as it is allocated is what makes the pages resident — Windows would
	// otherwise hand back address space the run never actually occupies.
	blocks := make([][]uint64, 0, budget/ramBlockBytes+1)
	defer func() {
		// Hand the memory back to Windows immediately rather than leaving the
		// scavenger to return it over the next few minutes — the user is
		// watching the RAM chart, and a run that ends should look like it did.
		blocks = nil
		debug.FreeOSMemory()
	}()

	rep.setPhase("allocating")
	for allocated := uint64(0); allocated < budget; {
		size := min(uint64(ramBlockBytes), budget-allocated)
		block := make([]uint64, size/8)
		for i := range block {
			block[i] = uint64(i)
		}
		blocks = append(blocks, block)
		allocated += size
		rep.set("Allocated", float64(allocated)/(1<<30), "GB")
		if ctx.Err() != nil {
			return nil
		}
	}
	if len(blocks) == 0 {
		return fmt.Errorf("not enough free memory to run a memory test")
	}

	var peak float64
	for pass := int64(1); ; pass++ {
		seed := uint64(pass) * 0x9E3779B97F4A7C15

		rep.setPhase("pass %d · writing pattern", pass)
		if !ramEach(ctx, blocks, func(b []uint64, base uint64) { ramFill(b, base, seed, false) }) {
			return nil
		}

		rep.setPhase("pass %d · verifying", pass)
		if !ramCheck(ctx, blocks, seed, false, rep) {
			return nil
		}

		// The inversion sweep reads and writes every word, so it is both the
		// bandwidth measurement and the setup for the second verification.
		rep.setPhase("pass %d · inverting", pass)
		start := time.Now()
		if !ramEach(ctx, blocks, ramInvert) {
			return nil
		}
		if secs := time.Since(start).Seconds(); secs > 0 {
			gbs := float64(budget) * 2 / secs / (1 << 30)
			peak = math.Max(peak, gbs)
			rep.set("Bandwidth", gbs, "GB/s")
			rep.set("Peak bandwidth", peak, "GB/s")
		}

		rep.setPhase("pass %d · verifying inverse", pass)
		if !ramCheck(ctx, blocks, seed, true, rep) {
			return nil
		}

		rep.setPhase("pass %d · random access", pass)
		if ns := ramChase(ctx, blocks[len(blocks)-1]); ns > 0 {
			rep.set("Random latency", ns, "ns")
		}

		rep.set("Passes", float64(pass), "")
		if ctx.Err() != nil {
			return nil
		}
	}
}

// ramCheck verifies every block against the expected pattern and folds any
// mismatches into the job's fault counter.
func ramCheck(ctx context.Context, blocks [][]uint64, seed uint64, inverted bool, rep *reporter) bool {
	var faults atomic.Int64
	alive := ramEach(ctx, blocks, func(b []uint64, base uint64) {
		faults.Add(ramVerify(b, base, seed, inverted))
	})
	rep.fault(faults.Load())
	return alive
}

// ramBudget decides how much to claim. The cap is on *available* memory, not
// total: the point is to fill what is free, not to push the machine into the
// page file and spend the run measuring the disk.
func ramBudget(percent int) (uint64, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, fmt.Errorf("reading memory state: %w", err)
	}
	budget := vm.Available * uint64(percent) / 100
	if vm.Available > ramReserveFree {
		if headroom := vm.Available - ramReserveFree; budget > headroom {
			budget = headroom
		}
	} else {
		budget = 0
	}

	budget = budget / ramBlockBytes * ramBlockBytes
	if budget == 0 {
		return 0, fmt.Errorf("only %.1f GB of %.1f GB is free — close some applications, "+
			"or lower the memory share so the test fits",
			float64(vm.Available)/(1<<30), float64(vm.Total)/(1<<30))
	}
	return budget, nil
}

// ramEach runs fn over every block in parallel, one goroutine per core, and
// reports whether the run is still alive. base is the block's word offset, so
// the pattern is a function of the global address.
func ramEach(ctx context.Context, blocks [][]uint64, fn func(block []uint64, base uint64)) bool {
	next := atomic.Int64{}
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(blocks) || ctx.Err() != nil {
					return
				}
				fn(blocks[i], uint64(i)*ramBlockBytes/8)
			}
		}()
	}
	wg.Wait()
	return ctx.Err() == nil
}

// ramPattern derives the expected word for an address. Mixing the address in
// means a stuck line, a mis-decoded row or an address-line fault all show up
// as a mismatch — a constant fill would only catch stuck bits.
func ramPattern(addr, seed uint64) uint64 {
	x := addr*0x9E3779B97F4A7C15 ^ seed
	x ^= x >> 29
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 32
	// Alternate the polarity every other word so neighbouring cells hold
	// opposing charges — the classic checkerboard that provokes coupling
	// faults between adjacent rows.
	if addr&1 == 0 {
		return x
	}
	return ^x
}

func ramFill(block []uint64, base, seed uint64, inverted bool) {
	for i := range block {
		want := ramPattern(base+uint64(i), seed)
		if inverted {
			want = ^want
		}
		block[i] = want
	}
}

func ramVerify(block []uint64, base, seed uint64, inverted bool) int64 {
	var faults int64
	for i := range block {
		want := ramPattern(base+uint64(i), seed)
		if inverted {
			want = ^want
		}
		if block[i] != want {
			faults++
		}
	}
	return faults
}

// ramInvert is the bandwidth sweep: every word is read, complemented and
// written back. Flipping every bit is also the worst case for the data lines,
// and it leaves the region in a state the next verification can check.
func ramInvert(block []uint64, _ uint64) {
	for i := range block {
		block[i] = ^block[i]
	}
}

// ramChase measures loaded random-access latency by walking a single random
// cycle through one block, so every step is a dependent miss the prefetcher
// cannot help with. It rewrites the block, which is why it runs after the
// pattern has been verified.
func ramChase(ctx context.Context, block []uint64) float64 {
	n := uint64(len(block))
	if n < 1<<20 {
		return 0
	}

	// A stride coprime to the length visits every slot exactly once, which
	// gives a full-coverage cycle without the cost of shuffling a permutation
	// of this size. ramBlockBytes is a power of two, so any odd stride will do.
	const stride = 0x9E3779B97F4A7C15 | 1
	for i, at := uint64(0), uint64(0); i < n; i++ {
		next := (at + stride) % n
		block[at] = next
		at = next
	}

	const steps = 4 << 20
	start := time.Now()
	at := uint64(0)
	for range steps {
		at = block[at]
	}
	elapsed := time.Since(start)
	cpuSink.Add(at)

	if ctx.Err() != nil {
		return 0
	}
	return float64(elapsed.Nanoseconds()) / steps
}
