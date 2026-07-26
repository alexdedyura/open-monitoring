//go:build !amd64

package stress

// Windows on amd64 is the supported target; everywhere else the portable
// scalar loop keeps the package compiling and still produces load.
func vectorKernel(bool) vecKernel {
	return vecKernel{"scalar FMA", fmaGeneric, fmaGenericFlops}
}
