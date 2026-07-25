//go:build windows

package metrics

import (
	"runtime"
	"sync"
)

// WMI/COM calls must run on a thread that stays initialised for COM. Go can
// migrate goroutines between OS threads at any time, so every query is funneled
// through one dedicated, OS-thread-locked worker. This also serialises the
// queries: the storage namespace is slow, and concurrent calls into it are a
// known source of multi-second stalls.
var (
	wmiOnce  sync.Once
	wmiQueue chan func()
)

func wmiWorker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The wmi package handles CoInitializeEx per call; keeping one thread for
	// the lifetime of the app avoids repeated init/uninit churn.
	for job := range wmiQueue {
		job()
	}
}

// runWMI executes fn on the dedicated WMI thread and waits for it to finish.
func runWMI(fn func()) {
	wmiOnce.Do(func() {
		wmiQueue = make(chan func(), 8)
		go wmiWorker()
	})
	done := make(chan struct{})
	wmiQueue <- func() {
		defer close(done)
		fn()
	}
	<-done
}
