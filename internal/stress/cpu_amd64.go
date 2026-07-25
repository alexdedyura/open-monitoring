//go:build amd64

package stress

import "golang.org/x/sys/cpu"

// The two assembly kernels below are the same loop at two register widths.
// Written in Go they would compile to the same instructions only by luck: the
// compiler does not auto-vectorise, so a stress test that wants the machine's
// real vector throughput has to ask for it directly.

//go:noescape
func fmaAVX2(iters int64, seed *[4]float64) float64

//go:noescape
func fmaAVX512(iters int64, seed *[4]float64) float64

const (
	fmaAVX2Flops   = fmaAccums * 4 * fmaFlopsOp // 4 doubles per YMM register
	fmaAVX512Flops = fmaAccums * 8 * fmaFlopsOp // 8 doubles per ZMM register
)

// vectorKernel picks the widest kernel the CPU supports. AVX-512 is opt-in
// because on many parts it drops the all-core clock hard — which is a genuine
// stress scenario, but not the one to run by default when the user only wants
// to know whether the cooling holds.
func vectorKernel(allowAVX512 bool) vecKernel {
	switch {
	case allowAVX512 && cpu.X86.HasAVX512F:
		return vecKernel{"AVX-512 FMA", fmaAVX512, fmaAVX512Flops}
	case cpu.X86.HasAVX2 && cpu.X86.HasFMA:
		return vecKernel{"AVX2 FMA", fmaAVX2, fmaAVX2Flops}
	}
	return vecKernel{"scalar FMA", fmaGeneric, fmaGenericFlops}
}
