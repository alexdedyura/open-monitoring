package stress

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/sha512"
	"math"
	"math/big"
	"math/bits"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// The CPU job is a power virus by design. A single kernel would only light up
// one part of the core — a pure FMA loop leaves the integer units idle, a pure
// integer loop never wakes the vector unit — and the chip would settle at a
// clock and a power draw that no real workload produces.
//
// So every thread rotates through the whole menu in short slices: wide FMA
// (AVX-512 or AVX2 where present), AES-NI plus the carry-less multiplier via
// GCM, the SHA-2 extensions, a long-latency integer dependency chain, bignum
// modular exponentiation, a cache/memory pointer chase, and the transcendental
// library. With the threads out of phase with each other, every execution port
// and both cache hierarchies stay busy for the whole run.
const (
	cpuSliceDuration = 120 * time.Millisecond // time spent in one kernel before rotating
	cpuScratchBytes  = 64 << 10               // AES/SHA working buffer
	cpuChaseBytes    = 24 << 20               // pointer-chase region: past L2, into L3 and RAM
)

func runCPU(ctx context.Context, o Options, rep *reporter) error {
	threads := o.CPUThreads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}

	vec := vectorKernel(o.CPUAVX512)
	rep.setDetail("%d threads · %s · AES-NI, SHA-2, bignum, cache and transcendental mix", threads, vec.name)
	rep.set("Vector throughput", 0, "GFLOPS")
	rep.set("Peak vector", 0, "GFLOPS")
	rep.set("Mixed work", 0, "Mops/s")

	var flops, vecNanos, ops atomic.Int64

	var wg sync.WaitGroup
	for i := range threads {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			newBurner(id, vec, &flops, &vecNanos, &ops).run(ctx)
		}(i)
	}

	// Report while the threads burn. The rates are windowed rather than
	// cumulative so the numbers track what the CPU is doing *now* — that is
	// what makes throttling visible as the package heats up.
	go func() {
		var peak float64
		lastFlops, lastVec, lastOps := int64(0), int64(0), int64(0)
		last := time.Now()

		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				dFlops := flops.Load() - lastFlops
				dVec := vecNanos.Load() - lastVec
				dOps := ops.Load() - lastOps
				lastFlops, lastVec, lastOps = flops.Load(), vecNanos.Load(), ops.Load()
				elapsed := now.Sub(last).Seconds()
				last = now

				// dVec sums the time every thread spent inside the vector
				// kernel; dividing by the thread count turns it back into the
				// wall time all of them were in it together.
				if dVec > 0 {
					gflops := float64(dFlops) * float64(threads) / float64(dVec)
					peak = math.Max(peak, gflops)
					rep.set("Vector throughput", gflops, "GFLOPS")
					rep.set("Peak vector", peak, "GFLOPS")
				}
				if elapsed > 0 {
					rep.set("Mixed work", float64(dOps)/elapsed/1e6, "Mops/s")
				}
			}
		}
	}()

	wg.Wait()
	return nil
}

// burner is one thread's private state. Everything it needs is allocated up
// front so the loop itself never touches the allocator — a stress kernel that
// spends its time in the GC is not stressing the CPU.
type burner struct {
	id  int
	vec vecKernel

	seed  [4]float64
	buf   []byte
	out   []byte
	ctr   cipher.Stream
	gcm   cipher.AEAD
	nonce []byte
	chase []uint32

	base, exp, mod, acc *big.Int

	flops, vecNanos, ops *atomic.Int64
	sink                 uint64
}

func newBurner(id int, vec vecKernel, flops, vecNanos, ops *atomic.Int64) *burner {
	rng := rand.New(rand.NewSource(int64(id)*0x9E3779B9 + 1))

	key := make([]byte, 32)
	rng.Read(key)
	block, _ := aes.NewCipher(key)
	iv := make([]byte, aes.BlockSize)
	rng.Read(iv)
	gcm, _ := cipher.NewGCM(block)

	b := &burner{
		id:  id,
		vec: vec,
		// Values chosen so the FMA chain converges instead of running off to
		// infinity: denormals and NaNs cost extra cycles on some cores and
		// would make the throughput number meaningless.
		seed:     [4]float64{1.0, 0.9999997, 1e-7, 0},
		buf:      make([]byte, cpuScratchBytes),
		out:      make([]byte, 0, cpuScratchBytes+64),
		ctr:      cipher.NewCTR(block, iv),
		gcm:      gcm,
		nonce:    make([]byte, gcm.NonceSize()),
		chase:    make([]uint32, cpuChaseBytes/4),
		base:     new(big.Int),
		exp:      new(big.Int),
		mod:      new(big.Int),
		acc:      new(big.Int),
		flops:    flops,
		vecNanos: vecNanos,
		ops:      ops,
	}
	rng.Read(b.buf)

	// A single random cycle through the region: every step depends on the
	// previous load, so the core cannot prefetch its way out of the latency.
	for i := range b.chase {
		b.chase[i] = uint32(i)
	}
	for i := len(b.chase) - 1; i > 0; i-- { // Sattolo — one cycle, no fixed points
		j := rng.Intn(i)
		b.chase[i], b.chase[j] = b.chase[j], b.chase[i]
	}

	b.base.SetBit(b.base, 1021, 1)
	b.base.Sub(b.base, big.NewInt(int64(id)*2+1))
	b.exp.SetBit(b.exp, 1019, 1)
	b.exp.Sub(b.exp, big.NewInt(7))
	b.mod.SetBit(b.mod, 1023, 1)
	b.mod.Sub(b.mod, big.NewInt(357)) // odd, so Exp takes the Montgomery path

	return b
}

// run rotates through the kernels until the job is cancelled. Threads start at
// different points in the rotation so the machine never has every core in the
// same kernel — that is what keeps all the ports loaded at once.
func (b *burner) run(ctx context.Context) {
	kernels := []func(*burner){
		(*burner).vectorFMA,
		(*burner).aesRound,
		(*burner).shaRound,
		(*burner).integerChain,
		(*burner).bignum,
		(*burner).pointerChase,
		(*burner).transcendental,
	}

	for i := b.id; ; i++ {
		if ctx.Err() != nil {
			cpuSink.Add(b.sink) // keep the results observable, and the work alive
			return
		}
		kernel := kernels[i%len(kernels)]
		deadline := time.Now().Add(cpuSliceDuration)
		for time.Now().Before(deadline) {
			kernel(b)
		}
	}
}

// cpuSink exists so the compiler cannot prove the kernels' results are unused.
var cpuSink atomic.Uint64

const vectorBatch = 200_000 // FMA rounds per call; ~1 ms of work

func (b *burner) vectorFMA() {
	start := time.Now()
	v := b.vec.fn(vectorBatch, &b.seed)
	b.vecNanos.Add(int64(time.Since(start)))
	b.flops.Add(vectorBatch * b.vec.flopsPerIter)
	b.sink += uint64(math.Float64bits(v))
}

// aesRound exercises AES-NI, and through GCM the carry-less multiplier too.
func (b *burner) aesRound() {
	b.ctr.XORKeyStream(b.buf, b.buf)
	b.out = b.gcm.Seal(b.out[:0], b.nonce, b.buf, nil)
	b.sink += uint64(b.out[0])
	b.ops.Add(int64(len(b.buf)) / 16 * 2)
}

// shaRound drives the SHA extensions (SHA-256) and the wide integer path that
// SHA-512 takes when they are absent.
func (b *burner) shaRound() {
	h256 := sha256.Sum256(b.buf)
	h512 := sha512.Sum512(b.buf)
	b.sink += uint64(h256[0]) + uint64(h512[0])
	b.ops.Add(int64(len(b.buf)) / 64 * 2)
}

// integerChain is a serial multiply/rotate/popcount dependency chain: no
// instruction-level parallelism to hide behind, so it pins the integer ALUs
// and the multiplier at their latency limit.
func (b *burner) integerChain() {
	x := b.sink | 1
	for range 20_000 {
		hi, lo := bits.Mul64(x, 0x9E3779B97F4A7C15)
		x = lo ^ bits.RotateLeft64(hi, 27)
		x += uint64(bits.OnesCount64(x))
		x ^= x >> 31
	}
	b.sink = x
	b.ops.Add(20_000 * 4)
}

// bignum runs 1024-bit modular exponentiation — a long chain of full-width
// multiplies that keeps the multiplier and L1 busy for a very long time.
func (b *burner) bignum() {
	b.acc.Exp(b.base, b.exp, b.mod)
	if w := b.acc.Bits(); len(w) > 0 {
		b.sink += uint64(w[0])
	}
	b.base.Add(b.acc, big.NewInt(2))
	b.ops.Add(1024)
}

// pointerChase walks a random cycle through a region larger than L2, so every
// step is a cache miss the core cannot prefetch. This is the kernel that heats
// the uncore and the memory controller rather than the cores.
func (b *burner) pointerChase() {
	i := uint32(b.sink) % uint32(len(b.chase))
	for range 100_000 {
		i = b.chase[i]
	}
	b.sink += uint64(i)
	b.ops.Add(100_000)
}

// transcendental hits the software math library: long sequences of dependent
// floating-point work with branches, which neither the FMA loop nor the
// integer chain covers.
func (b *burner) transcendental() {
	x := 1.0 + float64(b.sink&0xFF)/256
	for range 20_000 {
		x = math.Sqrt(math.Abs(math.Sin(x)*math.Exp(-x) + math.Log1p(x)))
		x += 1.000001
	}
	b.sink += uint64(math.Float64bits(x))
	b.ops.Add(20_000 * 4)
}
