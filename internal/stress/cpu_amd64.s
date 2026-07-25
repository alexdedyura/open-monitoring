//go:build amd64

#include "textflag.h"

// Both kernels run the same loop: eight accumulators, each its own dependency
// chain, updated with a fused multiply-add. Eight is enough to cover the ~4
// cycle FMA latency on every current core, so the loop retires one FMA per
// port per cycle and the measured rate is the part's real vector throughput.
//
// The constants come in through seed so the chain converges (a = a*m + c with
// m just under 1): an accumulator that ran off into infinity or denormals
// would add microcode penalties on some cores and make the number a lie.

// func fmaAVX2(iters int64, seed *[4]float64) float64
TEXT ·fmaAVX2(SB), NOSPLIT, $0-24
	MOVQ iters+0(FP), CX
	MOVQ seed+8(FP), AX

	VBROADCASTSD (AX), Y0    // accumulator start
	VBROADCASTSD 8(AX), Y8   // multiplier
	VBROADCASTSD 16(AX), Y9  // addend

	VMOVAPD Y0, Y1
	VMOVAPD Y0, Y2
	VMOVAPD Y0, Y3
	VMOVAPD Y0, Y4
	VMOVAPD Y0, Y5
	VMOVAPD Y0, Y6
	VMOVAPD Y0, Y7

	TESTQ CX, CX
	JLE   avx2reduce

avx2loop:
	VFMADD213PD Y9, Y8, Y0
	VFMADD213PD Y9, Y8, Y1
	VFMADD213PD Y9, Y8, Y2
	VFMADD213PD Y9, Y8, Y3
	VFMADD213PD Y9, Y8, Y4
	VFMADD213PD Y9, Y8, Y5
	VFMADD213PD Y9, Y8, Y6
	VFMADD213PD Y9, Y8, Y7
	DECQ CX
	JNZ  avx2loop

avx2reduce:
	VADDPD Y1, Y0, Y0
	VADDPD Y3, Y2, Y2
	VADDPD Y5, Y4, Y4
	VADDPD Y7, Y6, Y6
	VADDPD Y2, Y0, Y0
	VADDPD Y6, Y4, Y4
	VADDPD Y4, Y0, Y0

	MOVSD X0, ret+16(FP)
	VZEROUPPER
	RET

// func fmaAVX512(iters int64, seed *[4]float64) float64
TEXT ·fmaAVX512(SB), NOSPLIT, $0-24
	MOVQ iters+0(FP), CX
	MOVQ seed+8(FP), AX

	VBROADCASTSD (AX), Z0
	VBROADCASTSD 8(AX), Z8
	VBROADCASTSD 16(AX), Z9

	VMOVAPD Z0, Z1
	VMOVAPD Z0, Z2
	VMOVAPD Z0, Z3
	VMOVAPD Z0, Z4
	VMOVAPD Z0, Z5
	VMOVAPD Z0, Z6
	VMOVAPD Z0, Z7

	TESTQ CX, CX
	JLE   avx512reduce

avx512loop:
	VFMADD213PD Z9, Z8, Z0
	VFMADD213PD Z9, Z8, Z1
	VFMADD213PD Z9, Z8, Z2
	VFMADD213PD Z9, Z8, Z3
	VFMADD213PD Z9, Z8, Z4
	VFMADD213PD Z9, Z8, Z5
	VFMADD213PD Z9, Z8, Z6
	VFMADD213PD Z9, Z8, Z7
	DECQ CX
	JNZ  avx512loop

avx512reduce:
	VADDPD Z1, Z0, Z0
	VADDPD Z3, Z2, Z2
	VADDPD Z5, Z4, Z4
	VADDPD Z7, Z6, Z6
	VADDPD Z2, Z0, Z0
	VADDPD Z6, Z4, Z4
	VADDPD Z4, Z0, Z0

	MOVSD X0, ret+16(FP)
	VZEROUPPER
	RET
