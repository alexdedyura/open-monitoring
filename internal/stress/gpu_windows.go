//go:build windows

package stress

import (
	"context"
	"fmt"
	"math"
	"time"
	"unsafe"
)

// The GPU job stresses three things that fail for different reasons, and a
// test that only did the first would miss the two faults people actually hit:
//
//   - the shader array, with a dense fused-multiply-add loop (heat and power);
//   - the memory *interface*, by streaming the whole of VRAM through the chip
//     — this is what an unstable memory overclock trips over, not the ALUs;
//   - memory *capacity*, by claiming the card's VRAM and holding a verified
//     pattern in all of it, which is the only way to find a bad cell that a
//     small working set would never touch.
//
// Every dispatch is kept to a few hundred milliseconds: Windows resets a GPU
// whose driver has not returned within two seconds, and a stress test that
// blue-screens the display driver by design is not a stress test.
const (
	gpuChunkBytes   = 512 << 20 // one VRAM allocation
	gpuMinChunk     = 64 << 20  // below this, stop trying to allocate
	gpuTargetBurst  = 250 * time.Millisecond
	gpuAluItemsPerU = 2048 // work items per compute unit in the ALU kernel
)

// gpuKernels is compiled by the installed driver at run time, so one source
// covers every vendor. It mirrors the memory job in ram.go on purpose: fill a
// pattern, flip every bit (the timed read-and-write sweep), verify the
// inverse — so a fault is a real mismatch and not a missing barrier.
const gpuKernels = `
uint om_pattern(uint i, uint seed) {
    uint x = i * 2654435761u ^ seed;
    x ^= x >> 15; x *= 2246822519u; x ^= x >> 13;
    return (i & 1u) ? ~x : x;
}

uint4 om_pattern4(uint i4, uint seed) {
    uint b = i4 << 2;
    return (uint4)(om_pattern(b, seed), om_pattern(b + 1u, seed),
                   om_pattern(b + 2u, seed), om_pattern(b + 3u, seed));
}

// Eight independent chains keep the FMA units throughput-bound instead of
// waiting on the latency of a single dependent chain.
__kernel void om_alu(__global float *out, const uint rounds, const float seed) {
    const uint gid = get_global_id(0);
    const float m = 0.9999997f, c = 1.0e-7f;
    float a0 = seed + gid * 1e-7f;
    float a1 = a0 + 0.125f, a2 = a0 + 0.25f,  a3 = a0 + 0.375f;
    float a4 = a0 + 0.5f,   a5 = a0 + 0.625f, a6 = a0 + 0.75f, a7 = a0 + 0.875f;
    for (uint i = 0; i < rounds; i++) {
        a0 = fma(a0, m, c); a1 = fma(a1, m, c);
        a2 = fma(a2, m, c); a3 = fma(a3, m, c);
        a4 = fma(a4, m, c); a5 = fma(a5, m, c);
        a6 = fma(a6, m, c); a7 = fma(a7, m, c);
    }
    out[gid] = a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7;
}

__kernel void om_fill(__global uint4 *buf, const uint n4, const uint seed) {
    for (uint i = get_global_id(0); i < n4; i += get_global_size(0))
        buf[i] = om_pattern4(i, seed);
}

__kernel void om_flip(__global uint4 *buf, const uint n4) {
    for (uint i = get_global_id(0); i < n4; i += get_global_size(0))
        buf[i] = ~buf[i];
}

__kernel void om_check(__global const uint4 *buf, const uint n4, const uint seed,
                       __global uint *errs) {
    uint bad = 0;
    for (uint i = get_global_id(0); i < n4; i += get_global_size(0)) {
        uint4 want = ~om_pattern4(i, seed);
        uint4 got  = buf[i];
        if (got.x != want.x) bad++;
        if (got.y != want.y) bad++;
        if (got.z != want.z) bad++;
        if (got.w != want.w) bad++;
    }
    if (bad > 0) atomic_add(errs, bad);
}
`

// gpuBuffer is one VRAM allocation, sized in uint4 elements because that is
// what the kernels index.
type gpuBuffer struct {
	mem uintptr
	n4  uint32
}

func runGPU(ctx context.Context, o Options, rep *reporter) error {
	if err := openCLReady(); err != nil {
		return err
	}
	devices, err := gpuDevices()
	if err != nil {
		return err
	}

	// With both an integrated and a discrete GPU present, the one with more
	// memory of its own is the one worth stressing.
	dev := devices[0]
	for _, d := range devices[1:] {
		if d.globalMem > dev.globalMem {
			dev = d
		}
	}

	sess, err := newSession(dev, gpuKernels, "om_alu", "om_fill", "om_flip", "om_check")
	if err != nil {
		return err
	}
	defer sess.release()

	rep.setDetail("%s · %d compute units · %d MHz · %.1f GB VRAM · %s",
		dev.name, dev.units, dev.clockMHz, float64(dev.globalMem)/(1<<30), dev.version)
	rep.set("Compute", 0, "GFLOPS")
	rep.set("Peak compute", 0, "GFLOPS")
	rep.set("VRAM bandwidth", 0, "GB/s")
	rep.set("Peak bandwidth", 0, "GB/s")
	rep.set("VRAM filled", 0, "GB")
	rep.set("Passes", 0, "")

	aluGlobal := uint64(max(dev.units, 1)) * gpuAluItemsPerU
	aluOut, err := sess.alloc(aluGlobal * 4)
	if err != nil {
		return err
	}
	errBuf, err := sess.alloc(4)
	if err != nil {
		return err
	}

	rep.setPhase("claiming VRAM")
	buffers, filled := gpuClaim(ctx, sess, dev, o.GPUPercent, rep)
	if ctx.Err() != nil {
		return nil
	}
	if len(buffers) == 0 {
		return fmt.Errorf("could not allocate any VRAM on %s", dev.name)
	}

	rounds := uint32(64)
	var peakCompute, peakBandwidth float64

	for pass := int64(1); ; pass++ {
		seed := uint32(pass) * 2654435761

		// Shader array.
		rep.setPhase("pass %d · compute", pass)
		start := time.Now()
		if err := gpuALU(sess, aluOut, aluGlobal, rounds, float32(pass)); err != nil {
			return err
		}
		if elapsed := time.Since(start).Seconds(); elapsed > 0 {
			gflops := float64(aluGlobal) * float64(rounds) * fmaAccums * fmaFlopsOp / elapsed / 1e9
			peakCompute = math.Max(peakCompute, gflops)
			rep.set("Compute", gflops, "GFLOPS")
			rep.set("Peak compute", peakCompute, "GFLOPS")
			rounds = gpuAdaptRounds(rounds, elapsed)
		}
		if ctx.Err() != nil {
			return nil
		}

		// Memory: write the pattern across everything claimed.
		rep.setPhase("pass %d · VRAM pattern", pass)
		if err := gpuSweep(ctx, sess, buffers, "om_fill", seed); err != nil {
			return err
		}

		// Memory interface: read and write every byte, timed.
		rep.setPhase("pass %d · VRAM bandwidth", pass)
		start = time.Now()
		if err := gpuSweep(ctx, sess, buffers, "om_flip", 0); err != nil {
			return err
		}
		if elapsed := time.Since(start).Seconds(); elapsed > 0 {
			gbs := float64(filled) * 2 / elapsed / (1 << 30)
			peakBandwidth = math.Max(peakBandwidth, gbs)
			rep.set("VRAM bandwidth", gbs, "GB/s")
			rep.set("Peak bandwidth", peakBandwidth, "GB/s")
		}

		// Integrity: the flip means the expected value is the inverse.
		rep.setPhase("pass %d · VRAM verify", pass)
		if err := sess.writeUint32(errBuf, 0); err != nil {
			return err
		}
		if err := gpuVerify(ctx, sess, buffers, seed, errBuf); err != nil {
			return err
		}
		faults, err := sess.readUint32(errBuf)
		if err != nil {
			return err
		}
		rep.fault(int64(faults))

		rep.set("Passes", float64(pass), "")
		if ctx.Err() != nil {
			return nil
		}
	}
}

// gpuClaim allocates VRAM up to the requested share and writes to every
// allocation, because drivers hand out addresses lazily — a buffer that is
// never touched costs no memory and would make "VRAM filled" a fiction.
func gpuClaim(ctx context.Context, sess *clSession, dev clDevice, percent int, rep *reporter) ([]gpuBuffer, uint64) {
	budget := dev.globalMem * uint64(percent) / 100
	chunk := uint64(gpuChunkBytes)
	if dev.maxAlloc > 0 && dev.maxAlloc < chunk {
		chunk = dev.maxAlloc
	}
	chunk = chunk / 16 * 16 // whole uint4 elements

	var buffers []gpuBuffer
	var filled uint64
	for filled < budget && ctx.Err() == nil {
		size := min(chunk, budget-filled)
		if size < gpuMinChunk {
			break
		}
		size = size / 16 * 16

		mem, err := sess.alloc(size)
		if err != nil {
			break // the card is full; carry on with what was claimed
		}
		buf := gpuBuffer{mem: mem, n4: uint32(size / 16)}
		if err := gpuKernel(sess, "om_fill", buf, 0); err != nil {
			break
		}
		if err := sess.finish(); err != nil {
			break
		}

		buffers = append(buffers, buf)
		filled += size
		rep.set("VRAM filled", float64(filled)/(1<<30), "GB")
	}
	return buffers, filled
}

// gpuSweep runs one memory kernel over every claimed buffer, one dispatch per
// buffer so no single command outstays the driver's watchdog.
func gpuSweep(ctx context.Context, sess *clSession, buffers []gpuBuffer, kernel string, seed uint32) error {
	for _, buf := range buffers {
		if ctx.Err() != nil {
			return nil
		}
		if err := gpuKernel(sess, kernel, buf, seed); err != nil {
			return err
		}
		if err := sess.finish(); err != nil {
			return err
		}
	}
	return nil
}

// gpuKernel dispatches om_fill or om_flip against one buffer.
func gpuKernel(sess *clSession, name string, buf gpuBuffer, seed uint32) error {
	k := sess.kernels[name]
	if err := setArg(k, 0, unsafe.Sizeof(buf.mem), unsafe.Pointer(&buf.mem)); err != nil {
		return err
	}
	if err := setArg(k, 1, 4, unsafe.Pointer(&buf.n4)); err != nil {
		return err
	}
	if name == "om_fill" {
		if err := setArg(k, 2, 4, unsafe.Pointer(&seed)); err != nil {
			return err
		}
	}
	return sess.enqueue(k, gpuMemGlobal(buf.n4))
}

func gpuVerify(ctx context.Context, sess *clSession, buffers []gpuBuffer, seed uint32, errBuf uintptr) error {
	k := sess.kernels["om_check"]
	for _, buf := range buffers {
		if ctx.Err() != nil {
			return nil
		}
		if err := setArg(k, 0, unsafe.Sizeof(buf.mem), unsafe.Pointer(&buf.mem)); err != nil {
			return err
		}
		if err := setArg(k, 1, 4, unsafe.Pointer(&buf.n4)); err != nil {
			return err
		}
		if err := setArg(k, 2, 4, unsafe.Pointer(&seed)); err != nil {
			return err
		}
		if err := setArg(k, 3, unsafe.Sizeof(errBuf), unsafe.Pointer(&errBuf)); err != nil {
			return err
		}
		if err := sess.enqueue(k, gpuMemGlobal(buf.n4)); err != nil {
			return err
		}
		if err := sess.finish(); err != nil {
			return err
		}
	}
	return nil
}

func gpuALU(sess *clSession, out uintptr, global uint64, rounds uint32, seed float32) error {
	k := sess.kernels["om_alu"]
	if err := setArg(k, 0, unsafe.Sizeof(out), unsafe.Pointer(&out)); err != nil {
		return err
	}
	if err := setArg(k, 1, 4, unsafe.Pointer(&rounds)); err != nil {
		return err
	}
	if err := setArg(k, 2, 4, unsafe.Pointer(&seed)); err != nil {
		return err
	}
	if err := sess.enqueue(k, global); err != nil {
		return err
	}
	return sess.finish()
}

// gpuMemGlobal keeps enough work items in flight to saturate the memory system
// without launching one per element, which would make the loop overhead the
// thing being measured.
func gpuMemGlobal(n4 uint32) uint64 {
	const items = 1 << 20
	if uint64(n4) < items {
		return uint64(n4)
	}
	return items
}

// gpuAdaptRounds steers the burst length towards the watchdog-safe target, so
// the first pass calibrates itself to the card instead of guessing.
func gpuAdaptRounds(rounds uint32, elapsed float64) uint32 {
	target := gpuTargetBurst.Seconds()
	scaled := float64(rounds) * target / math.Max(elapsed, 1e-4)
	return uint32(math.Min(math.Max(scaled, 16), 1<<22))
}
