//go:build !windows

package metrics

// The performance-counter clock source is Windows-only; elsewhere the clock
// comes from the sensor helper. See cpuclock_windows.go.
type CPUClockSource struct{}

func StartCPUClock() *CPUClockSource { return nil }

func (c *CPUClockSource) MHz() float64 { return 0 }

func (c *CPUClockSource) Stop() {}
