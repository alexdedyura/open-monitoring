//go:build !windows

package metrics

type FPSSource struct{}

func StartFPS() *FPSSource { return nil }

func (f *FPSSource) Stop() {}

func (f *FPSSource) Metrics() *FPSMetrics { return nil }
