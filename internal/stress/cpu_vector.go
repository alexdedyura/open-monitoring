package stress

import "math"

// vecKernel is the widest fused-multiply-add loop this build can run on this
// CPU. The loop shape is identical everywhere — eight independent accumulators
// so the FMA units are throughput-bound rather than latency-bound — only the
// register width changes, which is what makes the reported GFLOPS comparable
// to a vendor's peak figure for the part.
type vecKernel struct {
	name         string
	fn           func(iters int64, seed *[4]float64) float64
	flopsPerIter int64
}

// The kernels take their constants through a seed array rather than embedding
// them, so the same assembly serves any tuning:
//
//	seed[0] accumulator start · seed[1] multiplier · seed[2] addend.
const (
	fmaAccums  = 8 // independent chains in the loop body
	fmaFlopsOp = 2 // a fused multiply-add retires a multiply and an add
)

// fmaGeneric is the portable fallback. math.FMA lowers to a single hardware
// FMA wherever Go has one, so this is a scalar version of the same loop rather
// than a different kernel.
func fmaGeneric(iters int64, seed *[4]float64) float64 {
	m, c := seed[1], seed[2]
	a0, a1, a2, a3 := seed[0], seed[0], seed[0], seed[0]
	a4, a5, a6, a7 := seed[0], seed[0], seed[0], seed[0]

	for range iters {
		a0 = math.FMA(a0, m, c)
		a1 = math.FMA(a1, m, c)
		a2 = math.FMA(a2, m, c)
		a3 = math.FMA(a3, m, c)
		a4 = math.FMA(a4, m, c)
		a5 = math.FMA(a5, m, c)
		a6 = math.FMA(a6, m, c)
		a7 = math.FMA(a7, m, c)
	}
	return a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7
}

const fmaGenericFlops = fmaAccums * 1 * fmaFlopsOp
