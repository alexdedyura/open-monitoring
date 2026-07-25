//go:build windows

package metrics

import (
	"sync"
	"testing"
	"time"
)

// The WMI worker serialises every query onto one OS thread. Concurrent callers
// must queue rather than deadlock, and a caller must never be left hanging.
func TestRunWMIConcurrentCallers(t *testing.T) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	ran := 0

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWMI(func() {
				mu.Lock()
				ran++
				mu.Unlock()
			})
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runWMI deadlocked with concurrent callers")
	}

	if ran != 20 {
		t.Fatalf("ran = %d, want 20", ran)
	}
}

// physicalDisks must return promptly and degrade gracefully: without elevation
// the SMART counters are unavailable, which is not an error condition.
func TestPhysicalDisksDoesNotHang(t *testing.T) {
	done := make(chan []DiskHealthView, 1)
	go func() { done <- physicalDisks() }()

	select {
	case disks := <-done:
		for _, d := range disks {
			if d.Model == "" {
				t.Error("drive reported with an empty model")
			}
			if d.LifePercent < 0 || d.LifePercent > 100 {
				t.Errorf("%s: implausible remaining life %v", d.Model, d.LifePercent)
			}
		}
		t.Logf("enumerated %d drive(s)", len(disks))
	case <-time.After(30 * time.Second):
		t.Fatal("physicalDisks hung")
	}
}
