// Package stress runs synthetic load against the machine's subsystems so the
// monitoring side has something worth watching: a CPU/RAM/GPU/disk burn-in that
// drives clocks, temperatures and power to their sustained limits.
//
// Each subsystem is an independent job. They can be started together or one at
// a time, they carry their own deadline, and they publish a live readout while
// they run — the frontend renders that next to the sensor values the collector
// is already producing.
//
// Nothing here needs a driver or a helper process: the CPU kernels are Go plus
// a small amd64 assembly core, memory and disk are plain Go, and the GPU is
// driven through the OpenCL runtime that every GPU vendor installs with its
// display driver (see gpu_windows.go).
package stress

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Target names one stressable subsystem. The values double as the keys the
// frontend uses, so they are part of the API.
const (
	CPU  = "cpu"
	RAM  = "ram"
	GPU  = "gpu"
	Disk = "disk"
)

// AllTargets is the canonical order the UI lists jobs in.
var AllTargets = []string{CPU, RAM, GPU, Disk}

// Options describes one requested run. A run may cover several targets; every
// target gets the same duration but its own tuning fields.
type Options struct {
	Targets []string `json:"targets"`
	Seconds int      `json:"seconds"` // 0 = run until stopped

	CPUThreads int  `json:"cpuThreads"` // 0 = one worker per logical core
	CPUAVX512  bool `json:"cpuAvx512"`  // use the AVX-512 kernel where the CPU has it

	RAMPercent int `json:"ramPercent"` // share of *available* memory to claim
	GPUPercent int `json:"gpuPercent"` // share of VRAM to fill

	DiskPath string `json:"diskPath"` // directory the test file is written in
	DiskMB   int    `json:"diskMb"`   // working-file size
}

// Defaults are deliberately hard but not reckless: enough load to reach a
// thermal steady state, while leaving the machine usable.
func Defaults() Options {
	return Options{
		Targets:    slices.Clone(AllTargets),
		Seconds:    300,
		CPUThreads: runtime.NumCPU(),
		CPUAVX512:  true,
		RAMPercent: 70,
		GPUPercent: 80,
		DiskPath:   os.TempDir(),
		DiskMB:     2048,
	}
}

// Clamp forces every tunable back into a range the workers can honour. It is
// the single guard against a hand-crafted request from the frontend.
func Clamp(o *Options) {
	if o.Seconds < 0 || o.Seconds > 24*3600 {
		o.Seconds = 0
	}
	if o.CPUThreads < 0 || o.CPUThreads > 4*runtime.NumCPU() {
		o.CPUThreads = runtime.NumCPU()
	}
	o.RAMPercent = clampInt(o.RAMPercent, 10, 90, 70)
	o.GPUPercent = clampInt(o.GPUPercent, 10, 95, 80)
	o.DiskMB = clampInt(o.DiskMB, 256, 64*1024, 2048)
	if o.DiskPath == "" {
		o.DiskPath = os.TempDir()
	}

	// Drop anything that is not a known target, and de-duplicate.
	kept := o.Targets[:0]
	for _, t := range o.Targets {
		if slices.Contains(AllTargets, t) && !slices.Contains(kept, t) {
			kept = append(kept, t)
		}
	}
	o.Targets = kept
}

func clampInt(v, lo, hi, def int) int {
	if v < lo || v > hi {
		return def
	}
	return v
}

// Readout is one live number a job publishes. Workers own their own set of
// labels — the UI just renders whatever arrives, in order.
type Readout struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Job states, in the order a job moves through them.
const (
	StateStarting = "starting"
	StateRunning  = "running"
	StateDone     = "done"
	StateStopped  = "stopped"
	StateFailed   = "failed"
)

// JobStatus is one subsystem's state as the frontend sees it. Finished jobs
// keep their last status so the results stay on screen after the run ends.
type JobStatus struct {
	Target    string    `json:"target"`
	State     string    `json:"state"`
	Phase     string    `json:"phase"`  // what the job is doing right now
	Detail    string    `json:"detail"` // one line describing the device/config
	Error     string    `json:"error"`
	Faults    int64     `json:"faults"` // data mismatches found by RAM/GPU verification
	StartedAt int64     `json:"startedAt"`
	EndsAt    int64     `json:"endsAt"` // unix ms; 0 when open-ended
	Stats     []Readout `json:"stats"`
}

// Status is the whole stress panel in one message.
type Status struct {
	Running bool        `json:"running"`
	Jobs    []JobStatus `json:"jobs"`
}

// job is one running subsystem worker.
type job struct {
	target    string
	cancel    context.CancelFunc
	startedAt int64
	endsAt    int64
	rep       *reporter
}

// Runner owns the running jobs and pushes a status snapshot once a second
// while anything is alive.
type Runner struct {
	mu      sync.Mutex
	jobs    map[string]*job
	last    map[string]JobStatus // final status of jobs that have finished
	pumping bool

	emit func(Status)
}

func NewRunner(emit func(Status)) *Runner {
	return &Runner{
		jobs: map[string]*job{},
		last: map[string]JobStatus{},
		emit: emit,
	}
}

var errNoTargets = errors.New("no stress targets selected")

// Start launches every requested target that is not already running. Targets
// already under load are left alone rather than restarted, so "run all" while
// the CPU job is up simply adds the rest.
func (r *Runner) Start(o Options) (Status, error) {
	Clamp(&o)
	if len(o.Targets) == 0 {
		return r.Status(), errNoTargets
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for _, target := range o.Targets {
		if _, busy := r.jobs[target]; busy {
			continue
		}
		kernel := kernelFor(target)
		if kernel == nil {
			continue
		}

		var ctx context.Context
		var cancel context.CancelFunc
		if o.Seconds > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), time.Duration(o.Seconds)*time.Second)
		} else {
			ctx, cancel = context.WithCancel(context.Background())
		}

		j := &job{
			target:    target,
			cancel:    cancel,
			startedAt: now.UnixMilli(),
			rep:       newReporter(),
		}
		if o.Seconds > 0 {
			j.endsAt = now.Add(time.Duration(o.Seconds) * time.Second).UnixMilli()
		}
		j.rep.setState(StateStarting)
		r.jobs[target] = j
		delete(r.last, target)

		go r.run(ctx, cancel, j, o, kernel)
	}

	r.startPumpLocked()
	return r.statusLocked(), nil
}

// run executes one job to completion and records how it ended.
func (r *Runner) run(ctx context.Context, cancel context.CancelFunc, j *job, o Options, k kernel) {
	defer cancel()

	// A worker that panics must not take the app down with it: report it like
	// any other failure and let the rest of the run continue.
	defer func() {
		if p := recover(); p != nil {
			j.rep.fail(fmt.Errorf("%v", p))
		}
		r.finish(j)
	}()

	j.rep.setState(StateRunning)
	if err := k(ctx, o, j.rep); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		j.rep.fail(err)
		return
	}

	// Reaching the deadline is a completed run; an explicit stop is not.
	if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		j.rep.setState(StateDone)
	} else {
		j.rep.setState(StateStopped)
	}
}

// finish moves a job out of the running set, keeping its last status visible.
func (r *Runner) finish(j *job) {
	r.mu.Lock()
	if cur, ok := r.jobs[j.target]; ok && cur == j {
		delete(r.jobs, j.target)
		r.last[j.target] = j.snapshot()
	}
	status := r.statusLocked()
	r.mu.Unlock()

	r.publish(status)
}

// Stop cancels the named targets, or every running job when the list is empty.
func (r *Runner) Stop(targets []string) Status {
	r.mu.Lock()
	for target, j := range r.jobs {
		if len(targets) == 0 || slices.Contains(targets, target) {
			j.rep.setState(StateStopped)
			j.cancel()
		}
	}
	status := r.statusLocked()
	r.mu.Unlock()
	return status
}

// StopAll cancels everything and waits briefly for the workers to unwind, so
// the disk job gets a chance to delete its test file on shutdown.
func (r *Runner) StopAll() {
	r.Stop(nil)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := len(r.jobs)
		r.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

func (r *Runner) statusLocked() Status {
	st := Status{Running: len(r.jobs) > 0}
	for _, target := range AllTargets {
		if j, ok := r.jobs[target]; ok {
			st.Jobs = append(st.Jobs, j.snapshot())
		} else if prev, ok := r.last[target]; ok {
			st.Jobs = append(st.Jobs, prev)
		}
	}
	return st
}

func (j *job) snapshot() JobStatus {
	s := j.rep.snapshot()
	s.Target = j.target
	s.StartedAt = j.startedAt
	s.EndsAt = j.endsAt
	return s
}

// startPumpLocked keeps a single ticker alive for as long as any job is
// running; the frontend redraws off the events it emits.
func (r *Runner) startPumpLocked() {
	if r.pumping || len(r.jobs) == 0 {
		return
	}
	r.pumping = true

	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for range tick.C {
			r.mu.Lock()
			status := r.statusLocked()
			if len(r.jobs) == 0 {
				r.pumping = false
				r.mu.Unlock()
				r.publish(status)
				return
			}
			r.mu.Unlock()
			r.publish(status)
		}
	}()
}

func (r *Runner) publish(s Status) {
	if r.emit != nil {
		r.emit(s)
	}
}

// kernel is the shape of a subsystem worker: run until the context ends, or
// return the reason it could not.
type kernel func(ctx context.Context, o Options, rep *reporter) error

func kernelFor(target string) kernel {
	switch target {
	case CPU:
		return runCPU
	case RAM:
		return runRAM
	case GPU:
		return runGPU
	case Disk:
		return runDisk
	}
	return nil
}

// reporter is the handle a worker publishes progress through. Everything on it
// is safe to call from any goroutine — the CPU and memory workers write from a
// fan-out of goroutines while the status pump reads.
type reporter struct {
	mu     sync.Mutex
	state  string
	phase  string
	detail string
	errMsg string
	stats  []Readout

	faults atomic.Int64
}

func newReporter() *reporter { return &reporter{state: StateStarting} }

func (r *reporter) setState(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A finished job never moves back to running.
	if r.state == StateFailed || r.state == StateDone {
		return
	}
	r.state = s
}

func (r *reporter) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = StateFailed
	r.errMsg = err.Error()
}

func (r *reporter) setPhase(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase = fmt.Sprintf(format, args...)
}

func (r *reporter) setDetail(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detail = fmt.Sprintf(format, args...)
}

// set upserts one readout by label, so a worker can refresh a number without
// the list reordering under the UI.
func (r *reporter) set(label string, value float64, unit string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.stats {
		if r.stats[i].Label == label {
			r.stats[i].Value = value
			return
		}
	}
	r.stats = append(r.stats, Readout{Label: label, Value: value, Unit: unit})
}

func (r *reporter) fault(n int64) { r.faults.Add(n) }

func (r *reporter) snapshot() JobStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return JobStatus{
		State:  r.state,
		Phase:  r.phase,
		Detail: r.detail,
		Error:  r.errMsg,
		Faults: r.faults.Load(),
		Stats:  slices.Clone(r.stats),
	}
}
