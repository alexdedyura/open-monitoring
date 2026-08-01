//go:build !windows

package metrics

// The process table comes from NtQuerySystemInformation, a Windows native API
// with no portable equivalent worth the code. See process_windows.go.
type ProcessSource struct{}

func StartProcesses() *ProcessSource { return nil }

func (p *ProcessSource) Snapshot() ProcessSnapshot { return ProcessSnapshot{} }

func (p *ProcessSource) Stop() {}
