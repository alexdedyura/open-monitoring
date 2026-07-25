//go:build windows

package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yusufpapurcu/wmi"
)

// Windows exposes the effective core frequency through a performance counter:
// ProcessorFrequency is the base clock and PercentProcessorPerformance is how
// far above (or below) it the core is currently running. That gives a real
// boost clock without the ring-0 MSR reads — and therefore without the
// WinRing0 driver.
type win32PerfProcInfo struct {
	Name                        string
	PercentProcessorPerformance uint64
	ProcessorFrequency          uint32
}

// CPUClockSource polls in the background: the counter query costs tens of
// milliseconds, which is too much to spend inside the sampling loop.
type CPUClockSource struct {
	mu     sync.Mutex
	mhz    float64
	cancel context.CancelFunc
}

func StartCPUClock() *CPUClockSource {
	ctx, cancel := context.WithCancel(context.Background())
	c := &CPUClockSource{cancel: cancel}
	go c.loop(ctx)
	return c
}

// MHz returns the highest core frequency seen in the last poll, 0 if unknown.
func (c *CPUClockSource) MHz() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mhz
}

func (c *CPUClockSource) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *CPUClockSource) loop(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		if v := queryPeakClock(); v > 0 {
			c.mu.Lock()
			c.mhz = v
			c.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// queryPeakClock reports the fastest core rather than the average: on hybrid
// CPUs the _Total instance blends P and E cores into a figure that matches
// neither, while overlays conventionally show the peak.
func queryPeakClock() float64 {
	var rows []win32PerfProcInfo
	q := "SELECT Name, PercentProcessorPerformance, ProcessorFrequency FROM Win32_PerfFormattedData_Counters_ProcessorInformation"

	var err error
	runWMI(func() {
		err = wmi.Query(q, &rows)
	})
	if err != nil {
		return 0
	}

	peak := 0.0
	for _, r := range rows {
		if strings.Contains(r.Name, "_Total") || r.ProcessorFrequency == 0 {
			continue
		}
		if mhz := float64(r.ProcessorFrequency) * float64(r.PercentProcessorPerformance) / 100; mhz > peak {
			peak = mhz
		}
	}
	return round1(peak)
}
