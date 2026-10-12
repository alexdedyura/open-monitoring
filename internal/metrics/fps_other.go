//go:build !windows

package metrics

// PresentMon is a Windows-only tool, so frame-rate metrics are absent
// elsewhere. See fps_windows.go.
type FPSSource struct{}

func StartFPS() *FPSSource { return nil }

func (f *FPSSource) Running() bool { return false }

func (f *FPSSource) LastError() string { return "" }

func (f *FPSSource) CanElevate() bool { return false }

func (f *FPSSource) RequestElevation() {}

func (f *FPSSource) Metrics() *FPSMetrics { return nil }

func (f *FPSSource) Reset() {}

func (f *FPSSource) Stop() {}

// FPSRelayArg is the command-line switch of the elevated PresentMon relay,
// which only exists on Windows.
const FPSRelayArg = "--fps-relay"

func RunFPSRelay(args []string) int { return 1 }

// RestoreFSO re-enables fullscreen optimisations; a Windows-only setting.
func RestoreFSO(kind, exe string) error { return nil }
