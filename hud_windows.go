//go:build windows

package main

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A frameless always-on-top window is not enough to sit above a
// borderless-fullscreen game: when the game takes focus it re-asserts its own
// Z-order and covers a one-shot topmost window. Lightweight overlays solve this
// by periodically re-applying HWND_TOPMOST (heavier ones inject a D3D hook).
// We take the periodic approach — reliable for borderless windowed; exclusive
// fullscreen bypasses the desktop entirely and is out of reach either way (the
// UI warns about that before switching to the HUD).

var (
	modUser32                = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows          = modUser32.NewProc("EnumWindows")
	procGetWindowThreadPID   = modUser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible      = modUser32.NewProc("IsWindowVisible")
	procGetWindow            = modUser32.NewProc("GetWindow")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procSetWindowPos         = modUser32.NewProc("SetWindowPos")
)

const (
	gwOwner       = 4
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST == (HWND)-1
)

// findMainWindow returns our process's single top-level, visible, titled
// window — the Wails main window (WebView2 runs in separate processes, so no
// child windows are enumerated here).
func findMainWindow() uintptr {
	var found uintptr
	pid := windows.GetCurrentProcessId()
	cb := windows.NewCallback(func(h uintptr, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1 // keep enumerating
		}
		if owner, _, _ := procGetWindow.Call(h, gwOwner); owner != 0 {
			return 1
		}
		if vis, _, _ := procIsWindowVisible.Call(h); vis == 0 {
			return 1
		}
		if tl, _, _ := procGetWindowTextLengthW.Call(h); tl == 0 {
			return 1
		}
		found = h
		return 0 // stop
	})
	procEnumWindows.Call(cb, 0)
	return found
}

type topmostKeeper struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

var keeper topmostKeeper

func (t *topmostKeeper) start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	go func() {
		hwnd := findMainWindow()
		tk := time.NewTicker(700 * time.Millisecond)
		defer tk.Stop()
		for {
			if hwnd == 0 {
				hwnd = findMainWindow()
			}
			if hwnd != 0 {
				// SWP_NOACTIVATE: never steal focus from the game while re-asserting.
				procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			}
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
			}
		}
	}()
}

func (t *topmostKeeper) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
}

func hudTopmostOn()  { keeper.start() }
func hudTopmostOff() { keeper.stop() }
