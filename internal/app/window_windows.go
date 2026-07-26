//go:build windows

package app

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
	modUser32                      = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows                = modUser32.NewProc("EnumWindows")
	procGetWindowThreadPID         = modUser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible            = modUser32.NewProc("IsWindowVisible")
	procGetWindow                  = modUser32.NewProc("GetWindow")
	procGetWindowTextLengthW       = modUser32.NewProc("GetWindowTextLengthW")
	procSetWindowPos               = modUser32.NewProc("SetWindowPos")
	procGetSystemMetrics           = modUser32.NewProc("GetSystemMetrics")
	procSystemParametersInfo       = modUser32.NewProc("SystemParametersInfoW")
	procGetWindowLongPtrW          = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW          = modUser32.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes = modUser32.NewProc("SetLayeredWindowAttributes")
)

const (
	gwOwner       = 4
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST == (HWND)-1

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	spiGetWorkArea = 0x0030

	gwlExStyle      = ^uintptr(19) // GWL_EXSTYLE == -20
	wsExTransparent = 0x00000020
	wsExLayered     = 0x00080000
	lwaAlpha        = 0x2
)

// applyClickThrough makes the window invisible to the mouse (or visible
// again): clicks land on whatever is underneath, which for the HUD is the
// game. WS_EX_TRANSPARENT only works on a layered window, and a layered
// window that never sets its attributes stops painting — hence the full-alpha
// attribute when the style has to be added.
func applyClickThrough(on bool) {
	hwnd := findMainWindow()
	if hwnd == 0 {
		return
	}
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	if !on {
		procSetWindowLongPtrW.Call(hwnd, gwlExStyle, style&^uintptr(wsExTransparent))
		return
	}
	if style&wsExLayered == 0 {
		procSetWindowLongPtrW.Call(hwnd, gwlExStyle, style|wsExLayered|wsExTransparent)
		procSetLayeredWindowAttributes.Call(hwnd, 0, 255, lwaAlpha)
	} else {
		procSetWindowLongPtrW.Call(hwnd, gwlExStyle, style|wsExTransparent)
	}
}

// minVisible is how much of the overlay must remain on screen for a saved
// position to be considered usable.
const minVisible = 80

// virtualScreen returns the bounding rectangle of all monitors. On a
// multi-monitor desktop its origin is negative when a monitor sits left of or
// above the primary one, so "negative" alone never means "off screen".
func virtualScreen() (x, y, w, h int) {
	get := func(index uintptr) int {
		v, _, _ := procGetSystemMetrics.Call(index)
		return int(int32(v))
	}
	return get(smXVirtualScreen), get(smYVirtualScreen),
		get(smCXVirtualScreen), get(smCYVirtualScreen)
}

type rect struct{ Left, Top, Right, Bottom int32 }

// workArea returns the primary monitor's usable area, excluding the taskbar.
func workArea() (x, y, w, h int) {
	var r rect
	ok, _, _ := procSystemParametersInfo.Call(
		spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ok == 0 {
		vx, vy, vw, vh := virtualScreen()
		return vx, vy, vw, vh
	}
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// ClampToScreen keeps a window rectangle reachable. A saved position can become
// unusable when monitors are unplugged or rearranged — the window is then still
// there, just painted where nobody can see it. If too little of it would remain
// visible, fall back to the top-left of the work area.
func ClampToScreen(x, y, w, h int) (int, int) {
	vx, vy, vw, vh := virtualScreen()
	if vw <= 0 || vh <= 0 {
		return x, y
	}

	visible := overlap(x, x+w, vx, vx+vw) >= minVisible &&
		overlap(y, y+h, vy, vy+vh) >= minVisible
	if visible {
		return x, y
	}

	const margin = 16
	wx, wy, _, _ := workArea()
	return wx + margin, wy + margin
}

func overlap(aStart, aEnd, bStart, bEnd int) int {
	start := max(aStart, bStart)
	end := min(aEnd, bEnd)
	return end - start
}

// AnchorPosition places a window of the given size in a corner of the primary
// monitor's work area.
func AnchorPosition(anchor string, w, h int) (int, int) {
	const margin = 16
	ax, ay, aw, ah := workArea()

	x, y := ax+margin, ay+margin
	if anchor == "tr" || anchor == "br" {
		x = ax + aw - w - margin
	}
	if anchor == "bl" || anchor == "br" {
		y = ay + ah - h - margin
	}
	return x, y
}

// The Go runtime can only ever create a bounded number of callbacks and never
// frees them, so the EnumWindows callback is built exactly once. (Creating it
// per call would exhaust the table and abort the process after enough polls.)
var (
	enumFoundHwnd uintptr
	enumPID       uint32
	enumCallback  = windows.NewCallback(func(h uintptr, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&wpid)))
		if wpid != enumPID {
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
		enumFoundHwnd = h
		return 0 // stop
	})
)

// findMainWindow returns our process's single top-level, visible, titled
// window — the Wails main window (WebView2 runs in separate processes, so no
// child windows are enumerated here). Called from the keeper goroutine and
// from click-through switches, so the shared enum state is locked.
var enumMu sync.Mutex

func findMainWindow() uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()

	enumFoundHwnd = 0
	enumPID = windows.GetCurrentProcessId()
	procEnumWindows.Call(enumCallback, 0)
	return enumFoundHwnd
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

func startTopmostKeeper() { keeper.start() }
func stopTopmostKeeper()  { keeper.stop() }
