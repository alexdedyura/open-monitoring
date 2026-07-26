//go:build !windows

package hotkey

// System-wide shortcuts are registered through the Win32 API, so elsewhere
// there is nothing to register. See hotkey_windows.go.

type Binding struct {
	Combo string
	Do    func()
}

type Manager struct{}

func Register([]Binding) (*Manager, []error) { return &Manager{}, nil }

func (m *Manager) Stop() {}
