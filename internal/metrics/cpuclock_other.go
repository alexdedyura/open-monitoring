//go:build !windows

package metrics

// CPUClockSource is Windows-only; elsewhere the clock comes from the optional
// LibreHardwareMonitor bridge, if it is enabled at all.
type CPUClockSource struct{}

func StartCPUClock() *CPUClockSource { return nil }

func (c *CPUClockSource) MHz() float64 { return 0 }

func (c *CPUClockSource) Stop() {}
