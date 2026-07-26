//go:build !windows

package hotkey

// System-wide shortcuts are registered through the Win32 API, so elsewhere
// there is nothing to register. See hotkey_windows.go.

type Binding struct {
	Combo string
	Do    func()
}

type Manager struct{}

func Register(bindings []Binding) (*Manager, []error) {
	return &Manager{}, make([]error, len(bindings))
}

func (m *Manager) Stop() {}
