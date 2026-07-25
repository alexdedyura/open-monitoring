//go:build windows

package stress

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A GPU stress test needs to run code on the GPU, and this app cannot use cgo
// (there is no C toolchain in the build), so the usual bindings are out. What
// is left is a plain C API reachable by syscall — and OpenCL is exactly that:
// no COM vtables like Direct3D, no vendor lock like CUDA, and OpenCL.dll is
// the one thing NVIDIA, AMD and Intel all drop into System32 with their
// display drivers. The kernels are compiled by the driver at run time, so the
// same source stresses whatever card is installed.
//
// This file is only the binding; the workload is in gpu_windows.go.

var (
	openCL = windows.NewLazySystemDLL("OpenCL.dll")

	procGetPlatformIDs      = openCL.NewProc("clGetPlatformIDs")
	procGetDeviceIDs        = openCL.NewProc("clGetDeviceIDs")
	procGetDeviceInfo       = openCL.NewProc("clGetDeviceInfo")
	procCreateContext       = openCL.NewProc("clCreateContext")
	procCreateCommandQueue  = openCL.NewProc("clCreateCommandQueue")
	procCreateProgram       = openCL.NewProc("clCreateProgramWithSource")
	procBuildProgram        = openCL.NewProc("clBuildProgram")
	procGetProgramBuildInfo = openCL.NewProc("clGetProgramBuildInfo")
	procCreateKernel        = openCL.NewProc("clCreateKernel")
	procCreateBuffer        = openCL.NewProc("clCreateBuffer")
	procSetKernelArg        = openCL.NewProc("clSetKernelArg")
	procEnqueueNDRange      = openCL.NewProc("clEnqueueNDRangeKernel")
	procEnqueueRead         = openCL.NewProc("clEnqueueReadBuffer")
	procEnqueueWrite        = openCL.NewProc("clEnqueueWriteBuffer")
	procFinish              = openCL.NewProc("clFinish")
	procReleaseMem          = openCL.NewProc("clReleaseMemObject")
	procReleaseKernel       = openCL.NewProc("clReleaseKernel")
	procReleaseProgram      = openCL.NewProc("clReleaseProgram")
	procReleaseQueue        = openCL.NewProc("clReleaseCommandQueue")
	procReleaseContext      = openCL.NewProc("clReleaseContext")
)

// Constants from cl.h. Only the handful this file uses are reproduced.
const (
	clSuccess = 0

	clDeviceTypeGPU = 1 << 2

	clDeviceName             = 0x102B
	clDeviceVendor           = 0x102C
	clDeviceVersion          = 0x102F
	clDeviceGlobalMemSize    = 0x101F
	clDeviceMaxMemAllocSize  = 0x1010
	clDeviceMaxComputeUnits  = 0x1002
	clDeviceMaxClockFreq     = 0x100C
	clProgramBuildLog        = 0x1183
	clMemReadWrite           = 1 << 0
	clTrue                   = 1
	clMemObjectAllocFailure  = -4
	clMemOutOfHostMemory     = -6
	clMemOutOfResources      = -5
	clDeviceGlobalMemCacheSz = 0x101E
)

// clError carries an OpenCL status code with the call that produced it.
type clError struct {
	op   string
	code int32
}

func (e clError) Error() string {
	return fmt.Sprintf("%s failed: %s", e.op, clErrorName(e.code))
}

func clErrorName(code int32) string {
	switch code {
	case -1:
		return "device not found"
	case -2:
		return "device not available"
	case -3:
		return "compiler not available"
	case clMemObjectAllocFailure:
		return "out of device memory"
	case clMemOutOfResources:
		return "out of resources"
	case clMemOutOfHostMemory:
		return "out of host memory"
	case -11:
		return "kernel build failed"
	case -30:
		return "invalid value"
	case -61:
		return "invalid buffer size"
	}
	return fmt.Sprintf("OpenCL error %d", code)
}

// openCLReady reports whether the ICD loader is present and exports
// everything this file calls, so no later call can panic on a missing symbol.
func openCLReady() error {
	if err := openCL.Load(); err != nil {
		return fmt.Errorf("no OpenCL runtime found — install or update the GPU driver")
	}
	for _, p := range []*windows.LazyProc{
		procGetPlatformIDs, procGetDeviceIDs, procGetDeviceInfo, procCreateContext,
		procCreateCommandQueue, procCreateProgram, procBuildProgram, procGetProgramBuildInfo,
		procCreateKernel, procCreateBuffer, procSetKernelArg, procEnqueueNDRange,
		procEnqueueRead, procEnqueueWrite, procFinish, procReleaseMem,
		procReleaseKernel, procReleaseProgram, procReleaseQueue, procReleaseContext,
	} {
		if err := p.Find(); err != nil {
			return fmt.Errorf("the installed OpenCL runtime is incomplete (%s missing)", p.Name)
		}
	}
	return nil
}

// clDevice is one GPU, with the properties the workload sizes itself against.
type clDevice struct {
	id        uintptr
	name      string
	vendor    string
	version   string
	globalMem uint64
	maxAlloc  uint64
	units     uint32
	clockMHz  uint32
}

// gpuDevices lists every GPU across every installed platform. A machine with
// an integrated and a discrete GPU exposes both, usually on separate
// platforms, so both loops are needed.
func gpuDevices() ([]clDevice, error) {
	var count uint32
	if r, _, _ := procGetPlatformIDs.Call(0, 0, uintptr(unsafe.Pointer(&count))); int32(r) != clSuccess {
		return nil, clError{"clGetPlatformIDs", int32(r)}
	}
	if count == 0 {
		return nil, fmt.Errorf("no OpenCL platform is installed")
	}

	platforms := make([]uintptr, count)
	if r, _, _ := procGetPlatformIDs.Call(uintptr(count), uintptr(unsafe.Pointer(&platforms[0])), 0); int32(r) != clSuccess {
		return nil, clError{"clGetPlatformIDs", int32(r)}
	}

	var devices []clDevice
	for _, p := range platforms {
		var n uint32
		if r, _, _ := procGetDeviceIDs.Call(p, clDeviceTypeGPU, 0, 0, uintptr(unsafe.Pointer(&n))); int32(r) != clSuccess || n == 0 {
			continue // a platform without a GPU (a CPU runtime, say) is not an error
		}
		ids := make([]uintptr, n)
		if r, _, _ := procGetDeviceIDs.Call(p, clDeviceTypeGPU, uintptr(n), uintptr(unsafe.Pointer(&ids[0])), 0); int32(r) != clSuccess {
			continue
		}
		for _, id := range ids {
			devices = append(devices, clDevice{
				id:        id,
				name:      strings.TrimSpace(deviceString(id, clDeviceName)),
				vendor:    strings.TrimSpace(deviceString(id, clDeviceVendor)),
				version:   strings.TrimSpace(deviceString(id, clDeviceVersion)),
				globalMem: deviceUint64(id, clDeviceGlobalMemSize),
				maxAlloc:  deviceUint64(id, clDeviceMaxMemAllocSize),
				units:     deviceUint32(id, clDeviceMaxComputeUnits),
				clockMHz:  deviceUint32(id, clDeviceMaxClockFreq),
			})
		}
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("the OpenCL runtime reports no GPU device")
	}
	return devices, nil
}

func deviceString(id uintptr, param uintptr) string {
	var size uintptr
	if r, _, _ := procGetDeviceInfo.Call(id, param, 0, 0, uintptr(unsafe.Pointer(&size))); int32(r) != clSuccess || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if r, _, _ := procGetDeviceInfo.Call(id, param, size, uintptr(unsafe.Pointer(&buf[0])), 0); int32(r) != clSuccess {
		return ""
	}
	return strings.TrimRight(string(buf), "\x00")
}

func deviceUint64(id uintptr, param uintptr) uint64 {
	var v uint64
	procGetDeviceInfo.Call(id, param, 8, uintptr(unsafe.Pointer(&v)), 0)
	return v
}

func deviceUint32(id uintptr, param uintptr) uint32 {
	var v uint32
	procGetDeviceInfo.Call(id, param, 4, uintptr(unsafe.Pointer(&v)), 0)
	return v
}

// clSession bundles the objects that live for the whole run, so the workload
// can release them with a single defer.
type clSession struct {
	ctx     uintptr
	queue   uintptr
	program uintptr
	kernels map[string]uintptr
	buffers []uintptr
}

func newSession(dev clDevice, source string, kernelNames ...string) (*clSession, error) {
	var code int32

	ctx, _, _ := procCreateContext.Call(0, 1, uintptr(unsafe.Pointer(&dev.id)), 0, 0, uintptr(unsafe.Pointer(&code)))
	if ctx == 0 {
		return nil, clError{"clCreateContext", code}
	}
	s := &clSession{ctx: ctx, kernels: map[string]uintptr{}}

	queue, _, _ := procCreateCommandQueue.Call(ctx, dev.id, 0, uintptr(unsafe.Pointer(&code)))
	if queue == 0 {
		s.release()
		return nil, clError{"clCreateCommandQueue", code}
	}
	s.queue = queue

	src, err := windows.BytePtrFromString(source)
	if err != nil {
		s.release()
		return nil, err
	}
	strs := [1]*byte{src}
	prog, _, _ := procCreateProgram.Call(ctx, 1, uintptr(unsafe.Pointer(&strs[0])), 0, uintptr(unsafe.Pointer(&code)))
	runtime.KeepAlive(src)
	if prog == 0 {
		s.release()
		return nil, clError{"clCreateProgramWithSource", code}
	}
	s.program = prog

	// -cl-fast-relaxed-math lets the driver keep the FMA chain in the form
	// the kernel is written in; without it some compilers spill it to memory
	// and the test measures the cache instead of the ALUs.
	opts, _ := windows.BytePtrFromString("-cl-fast-relaxed-math -cl-mad-enable")
	r, _, _ := procBuildProgram.Call(prog, 1, uintptr(unsafe.Pointer(&dev.id)), uintptr(unsafe.Pointer(opts)), 0, 0)
	runtime.KeepAlive(opts)
	if int32(r) != clSuccess {
		log := buildLog(prog, dev.id)
		s.release()
		return nil, fmt.Errorf("the GPU driver could not compile the stress kernels: %s", firstLine(log))
	}

	for _, name := range kernelNames {
		n, _ := windows.BytePtrFromString(name)
		k, _, _ := procCreateKernel.Call(prog, uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&code)))
		runtime.KeepAlive(n)
		if k == 0 {
			s.release()
			return nil, clError{"clCreateKernel " + name, code}
		}
		s.kernels[name] = k
	}
	return s, nil
}

func buildLog(prog, dev uintptr) string {
	var size uintptr
	procGetProgramBuildInfo.Call(prog, dev, clProgramBuildLog, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	procGetProgramBuildInfo.Call(prog, dev, clProgramBuildLog, size, uintptr(unsafe.Pointer(&buf[0])), 0)
	return strings.TrimRight(string(buf), "\x00")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no diagnostic returned"
	}
	if i := strings.IndexAny(s, "\r\n"); i > 0 {
		return s[:i]
	}
	return s
}

// alloc creates a device buffer and registers it for release.
func (s *clSession) alloc(size uint64) (uintptr, error) {
	var code int32
	m, _, _ := procCreateBuffer.Call(s.ctx, clMemReadWrite, uintptr(size), 0, uintptr(unsafe.Pointer(&code)))
	if m == 0 {
		return 0, clError{"clCreateBuffer", code}
	}
	s.buffers = append(s.buffers, m)
	return m, nil
}

// setArgs assigns a kernel's arguments. Buffers are passed as their handle,
// scalars by pointer to a local copy — which is what cl_kernel expects.
func setArg(kernel uintptr, index int, size uintptr, value unsafe.Pointer) error {
	r, _, _ := procSetKernelArg.Call(kernel, uintptr(index), size, uintptr(value))
	if int32(r) != clSuccess {
		return clError{fmt.Sprintf("clSetKernelArg(%d)", index), int32(r)}
	}
	return nil
}

// enqueue launches a 1-D kernel over globalSize work items, letting the driver
// pick the work-group size — vendors disagree about what is legal there, and
// getting it wrong is a hard error rather than a slow kernel.
func (s *clSession) enqueue(kernel uintptr, globalSize uint64) error {
	g := uintptr(globalSize)
	r, _, _ := procEnqueueNDRange.Call(s.queue, kernel, 1, 0, uintptr(unsafe.Pointer(&g)), 0, 0, 0, 0)
	if int32(r) != clSuccess {
		return clError{"clEnqueueNDRangeKernel", int32(r)}
	}
	return nil
}

func (s *clSession) finish() error {
	r, _, _ := procFinish.Call(s.queue)
	if int32(r) != clSuccess {
		return clError{"clFinish", int32(r)}
	}
	return nil
}

func (s *clSession) writeUint32(buf uintptr, v uint32) error {
	r, _, _ := procEnqueueWrite.Call(s.queue, buf, clTrue, 0, 4, uintptr(unsafe.Pointer(&v)), 0, 0, 0)
	if int32(r) != clSuccess {
		return clError{"clEnqueueWriteBuffer", int32(r)}
	}
	return nil
}

func (s *clSession) readUint32(buf uintptr) (uint32, error) {
	var v uint32
	r, _, _ := procEnqueueRead.Call(s.queue, buf, clTrue, 0, 4, uintptr(unsafe.Pointer(&v)), 0, 0, 0)
	if int32(r) != clSuccess {
		return 0, clError{"clEnqueueReadBuffer", int32(r)}
	}
	return v, nil
}

func (s *clSession) release() {
	for _, m := range s.buffers {
		procReleaseMem.Call(m)
	}
	for _, k := range s.kernels {
		procReleaseKernel.Call(k)
	}
	if s.program != 0 {
		procReleaseProgram.Call(s.program)
	}
	if s.queue != 0 {
		procReleaseQueue.Call(s.queue)
	}
	if s.ctx != 0 {
		procReleaseContext.Call(s.ctx)
	}
}
