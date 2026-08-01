//go:build windows

package metrics

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v4/cpu"
	"golang.org/x/sys/windows"
)

// processInterval is how often the table is rebuilt. It is deliberately a clock
// of its own rather than the sampling interval: enumerating is cheap but not
// free, per-process CPU is a delta that wants a stable window either side of
// it, and the sampling interval is a user setting that goes down to 250 ms. Two
// seconds is also about as fast as three hundred rows can reshuffle and still
// be readable.
const processInterval = 2 * time.Second

// spiSize bounds the walk over the returned data, so an entry offset that
// points past the end is never dereferenced.
const spiSize = int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))

// processBufSize is the starting size of the enumeration buffer. It is sized
// against threads, not processes: the kernel appends a SYSTEM_THREAD_INFORMATION
// array after every process entry, and on an ordinary desktop those outweigh the
// process structs about six to one — 265 processes measured here wanted 598 KB,
// of which the process entries were 66 KB. Guessing from the process count is
// how this ends up reallocating on the very first call.
const processBufSize = 1024 * 1024

// procKey identifies a process from one tick to the next. The PID alone will
// not do: Windows hands PIDs out again quickly, and a new process inheriting
// the dead one's CPU baseline reads as a nonsense spike. CreateTime comes back
// in the same struct, so keying on the pair costs nothing.
type procKey struct {
	pid     uint32
	created int64
}

// procPrev is the previous tick's cumulative counters for one process.
type procPrev struct {
	cpu100ns uint64
	readB    uint64
	writeB   uint64
}

// ProcessSource keeps a table of every running process refreshed in the
// background. One NtQuerySystemInformation call per tick covers the name, PID,
// thread count, CPU time, working set, private commit and cumulative I/O of all
// of them without opening a single process handle — which matters twice over,
// because OpenProcess is among the most heavily hooked calls on a machine
// running anti-virus, and a program that opens two thousand process handles a
// second looks exactly like what a heuristic scanner is hunting for.
type ProcessSource struct {
	cancel context.CancelFunc
	cpus   int // logical processors; the denominator for CPU %

	mu      sync.Mutex
	rows    []ProcessRow
	at      time.Time
	threads int
	err     string

	// Touched only by the loop goroutine, so they need no lock.
	prev   map[procKey]procPrev
	prevAt time.Time
	buf    []byte
}

// StartProcesses begins enumerating in the background. Like every source here
// it cannot fail: a machine where the call does not work simply serves an empty
// table with the reason attached.
func StartProcesses() *ProcessSource {
	// cpu.Counts(true) goes through GetActiveProcessorCount(ALL_PROCESSOR_GROUPS)
	// and is the number wanted here. runtime.NumCPU is only the fallback because
	// it counts the processor group this process is affinitized to: past 64
	// logical cores Windows splits the machine into groups, and taking that as
	// the denominator would report every row at double.
	n, err := cpu.Counts(true)
	if err != nil || n <= 0 {
		n = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &ProcessSource{
		cancel: cancel,
		cpus:   n,
		prev:   map[procKey]procPrev{},
		buf:    make([]byte, processBufSize),
	}
	go p.loop(ctx)
	return p
}

// Snapshot returns the most recent table. The rows are copied because the loop
// is free to replace them while the UI is reading.
func (p *ProcessSource) Snapshot() ProcessSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := ProcessSnapshot{
		Rows:    make([]ProcessRow, len(p.rows)),
		Threads: p.threads,
		Error:   p.err,
	}
	copy(out.Rows, p.rows)
	if !p.at.IsZero() {
		out.At = p.at.UnixMilli()
	}
	return out
}

func (p *ProcessSource) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

// loop refreshes first and waits afterwards, so a table is ready before anyone
// can open the tab. That first one carries no CPU or I/O rates — there is
// nothing yet to subtract from — and is published anyway, because the memory
// and thread columns are already right.
func (p *ProcessSource) loop(ctx context.Context) {
	t := time.NewTicker(processInterval)
	defer t.Stop()
	for {
		// Checked before the work, not only after it. select picks at random
		// among ready cases, so testing cancellation only at the bottom of the
		// loop leaves a coin flip on whether Stop() is followed by one more full
		// enumeration — and if a refresh ever overruns the interval the tick is
		// permanently ready, which makes it a coin flip every round.
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.refresh(time.Now())

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// refresh rebuilds the whole table from one enumeration. A failure leaves the
// last good table on screen with the reason beside it, rather than blanking a
// list the user was reading.
func (p *ProcessSource) refresh(now time.Time) {
	raw, err := p.query()
	if err != nil {
		p.mu.Lock()
		p.err = err.Error()
		p.mu.Unlock()
		return
	}

	// The measured gap, not processInterval: a ticker that was delayed, or a
	// query that failed for a few rounds, would otherwise divide a long
	// accumulation by a short window and report several hundred percent.
	elapsed := now.Sub(p.prevAt).Seconds()
	rated := !p.prevAt.IsZero() && elapsed > 0

	rows := make([]ProcessRow, 0, len(p.prev)+32)
	fresh := make(map[procKey]procPrev, len(p.prev)+32)
	threads := 0

	for off := 0; off+spiSize <= len(raw); {
		e := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&raw[off]))
		pid := uint32(e.UniqueProcessID)

		// PID 0 is the idle process, and its "CPU time" is the machine's idle
		// time — listing it would pin a permanent ninety-percent row to the top
		// of the table. PID 4 is the kernel and is a real, interesting row.
		if pid != 0 {
			key := procKey{pid: pid, created: e.CreateTime}
			cpu100ns := uint64(e.UserTime) + uint64(e.KernelTime)
			readB, writeB := uint64(e.ReadTransferCount), uint64(e.WriteTransferCount)

			row := ProcessRow{
				PID:       int(pid),
				Name:      processName(pid, e.ImageName),
				MemBytes:  uint64(e.WorkingSetSize),
				PrivBytes: uint64(e.PrivatePageCount),
				Threads:   int(e.NumberOfThreads),
			}
			if prev, ok := p.prev[key]; ok && rated {
				row.CPU = cpuPercent(delta(cpu100ns, prev.cpu100ns), elapsed, p.cpus)
				row.ReadBps = ratePerSec(readB, prev.readB, elapsed)
				row.WriteBps = ratePerSec(writeB, prev.writeB, elapsed)
			}

			fresh[key] = procPrev{cpu100ns: cpu100ns, readB: readB, writeB: writeB}
			threads += row.Threads
			rows = append(rows, row)
		}

		if e.NextEntryOffset == 0 {
			break
		}
		off += int(e.NextEntryOffset)
	}

	// Sorted here so the table has one deterministic order even before the
	// frontend applies whichever column the user picked.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CPU != rows[j].CPU {
			return rows[i].CPU > rows[j].CPU
		}
		return rows[i].PID < rows[j].PID
	})

	p.prev, p.prevAt = fresh, now

	p.mu.Lock()
	p.rows, p.at, p.threads, p.err = rows, now, threads, ""
	p.mu.Unlock()
}

// query fills the buffer with one snapshot of the process table, growing it
// until the call fits. This is a retry loop rather than the usual
// measure-then-read pair because processes start between the two calls: the
// length the kernel reported a moment ago is only ever a hint.
// The bound exists only so a pathological machine cannot spin here forever;
// converging takes one retry in practice, and the buffer is kept between ticks
// so even that happens once.
func (p *ProcessSource) query() ([]byte, error) {
	for attempt := 0; attempt < 6; attempt++ {
		var need uint32
		err := windows.NtQuerySystemInformation(
			windows.SystemProcessInformation,
			unsafe.Pointer(&p.buf[0]),
			uint32(len(p.buf)),
			&need,
		)
		if err == nil {
			// Only what the kernel just wrote. The buffer is reused and never
			// shrinks, so anything past `need` is a previous, larger enumeration
			// — bounding the walk by len(p.buf) would be walking stale entries.
			return p.buf[:need], nil
		}
		if err != windows.STATUS_INFO_LENGTH_MISMATCH {
			return nil, err
		}

		size := int(need) + 64*1024 // headroom for whatever starts next
		if grown := len(p.buf) * 2; grown > size {
			size = grown
		}
		p.buf = make([]byte, size)
	}
	return nil, errors.New("process table kept growing between reads")
}

// processName decodes the image name, which points into the buffer the kernel
// just filled. Length is a byte count, hence the halving.
//
// The nil-Buffer guard is load-bearing rather than defensive: every enumeration
// carries at least one entry without a name — PID 0, which the caller already
// drops — and kernel-owned entries can arrive the same way. They are still worth
// listing, so an unnamed process falls back to its PID.
func processName(pid uint32, s windows.NTUnicodeString) string {
	if s.Buffer != nil && s.Length > 0 {
		if name := windows.UTF16ToString(unsafe.Slice(s.Buffer, s.Length/2)); name != "" {
			return name
		}
	}
	if pid == 4 {
		return "System"
	}
	return "pid " + strconv.Itoa(int(pid))
}

// cpuPercent converts a CPU-time delta in 100-ns ticks into a share of the
// whole machine, the way Task Manager counts it: a process saturating every
// logical core reads 100, not 100 × cores. (htop's convention is the same
// number multiplied by the core count, but the comparison users actually make
// is against Task Manager.)
func cpuPercent(delta100ns uint64, elapsed float64, cpus int) float64 {
	if elapsed <= 0 || cpus <= 0 {
		return 0
	}
	pct := float64(delta100ns) / 1e7 / elapsed / float64(cpus) * 100
	if pct <= 0 {
		return 0
	}
	if pct > 100 {
		return 100 // clock skew, or a core count that does not match the one the kernel accounts against
	}
	return round1(pct)
}

func ratePerSec(cur, prev uint64, elapsed float64) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(delta(cur, prev)) / elapsed
}

// delta subtracts two readings of a counter that only ever climbs. Keying on
// (PID, CreateTime) already rules out the obvious way one could go backwards —
// a recycled PID bringing another process's accumulator with it — but the
// subtraction is unsigned, so anything that slipped through would report as
// eighteen quintillion rather than as a small mistake.
func delta(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}
