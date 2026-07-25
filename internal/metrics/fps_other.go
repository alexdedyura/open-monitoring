//go:build !windows

package metrics

// PresentMon is a Windows-only tool, so frame-rate metrics are absent
// elsewhere. See fps_windows.go.
type FPSSource struct{}

func StartFPS() *FPSSource { return nil }

func (f *FPSSource) Running() bool { return false }

func (f *FPSSource) Metrics() *FPSMetrics { return nil }

func (f *FPSSource) Stop() {}
