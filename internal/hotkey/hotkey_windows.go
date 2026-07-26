//go:build windows

// Package hotkey registers system-wide keyboard shortcuts.
//
// The HUD is an overlay: while it is useful the game owns the keyboard, so a
// shortcut that only works when the app has focus would never fire. Windows
// offers exactly one way to be heard anyway — RegisterHotKey — which claims the
// combination process-wide and delivers WM_HOTKEY to a thread message queue.
// That queue is the reason for the dedicated OS thread below.
package hotkey

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Modifier bits accepted by RegisterHotKey.
const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000 // fire once per press, not once per repeat
)

const wmHotkey = 0x0312

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessage       = user32.NewProc("GetMessageW")
	procPostThreadMsg    = user32.NewProc("PostThreadMessageW")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentTID    = kernel32.NewProc("GetCurrentThreadId")
)

// Binding is one shortcut and what it does.
type Binding struct {
	Combo string // "Ctrl+Alt+H"
	Do    func()
}

// Manager owns the message-loop thread the bindings are delivered on.
type Manager struct {
	tid  uint32
	quit chan struct{}
}

type msg struct {
	hwnd     windows.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       struct{ x, y int32 }
	lPrivate uint32
}

// Register claims every binding and starts listening. Bindings that Windows
// refuses — almost always because another application already owns the
// combination — are reported by their index; the rest still work.
func Register(bindings []Binding) (*Manager, []error) {
	m := &Manager{quit: make(chan struct{})}
	ready := make(chan []error, 1)

	go func() {
		// RegisterHotKey associates the shortcut with the calling thread, and
		// only that thread's queue receives WM_HOTKEY — so the registration and
		// the loop have to happen on one thread that never changes underneath
		// us. Nothing here ever blocks, so parking an OS thread costs nothing.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tid, _, _ := procGetCurrentTID.Call()
		m.tid = uint32(tid)

		var errs []error
		registered := make([]int, 0, len(bindings))
		for i, b := range bindings {
			mods, vk, err := parse(b.Combo)
			if err != nil {
				errs = append(errs, fmt.Errorf("hotkey %d (%q): %w", i, b.Combo, err))
				continue
			}
			ok, _, callErr := procRegisterHotKey.Call(0, uintptr(i+1), uintptr(mods|modNoRepeat), uintptr(vk))
			if ok == 0 {
				errs = append(errs, fmt.Errorf("hotkey %d (%q) is unavailable: %w", i, b.Combo, callErr))
				continue
			}
			registered = append(registered, i)
		}
		ready <- errs

		defer func() {
			for _, i := range registered {
				procUnregisterHotKey.Call(0, uintptr(i+1))
			}
			close(m.quit)
		}()

		// GetMessage blocks until something arrives and returns 0 on WM_QUIT,
		// which is what Stop posts.
		var received msg
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&received)), 0, 0, 0)
			if int32(ret) <= 0 {
				return
			}
			if received.message != wmHotkey {
				continue
			}
			if i := int(received.wParam) - 1; i >= 0 && i < len(bindings) && bindings[i].Do != nil {
				// The handler runs on the app's goroutines, not on the message
				// loop: a slow one would otherwise delay every later press.
				go bindings[i].Do()
			}
		}
	}()

	return m, <-ready
}

// Stop releases the shortcuts and ends the loop.
func (m *Manager) Stop() {
	if m == nil || m.tid == 0 {
		return
	}
	const wmQuit = 0x0012
	procPostThreadMsg.Call(uintptr(m.tid), wmQuit, 0, 0)
	<-m.quit
}

// parse turns "Ctrl+Alt+H" into the modifier bits and virtual-key code
// RegisterHotKey wants. Only single characters, digits and F-keys are
// supported, which covers everything worth binding an overlay to.
func parse(combo string) (mods uint32, vk uint32, err error) {
	parts := strings.Split(combo, "+")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("empty")
	}

	for _, raw := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "ctrl", "control":
			mods |= modControl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "win", "super":
			mods |= modWin
		default:
			return 0, 0, fmt.Errorf("unknown modifier %q", raw)
		}
	}

	key := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	switch {
	case len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9'):
		return mods, uint32(key[0]), nil // VK codes match ASCII for these
	case len(key) >= 2 && key[0] == 'F':
		n := 0
		if _, scanErr := fmt.Sscanf(key[1:], "%d", &n); scanErr != nil || n < 1 || n > 24 {
			return 0, 0, fmt.Errorf("unknown key %q", key)
		}
		return mods, uint32(0x70 + n - 1), nil // VK_F1 = 0x70
	}
	return 0, 0, fmt.Errorf("unknown key %q", key)
}
