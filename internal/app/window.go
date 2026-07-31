package app

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/config"
)

// window remembers the dashboard's geometry while the HUD is showing, so
// leaving the overlay restores the window exactly where it was.
//
// The app deliberately has one window rather than two: a second always-on-top
// window would need its own WebView2 instance, doubling memory for a view that
// renders the same data.
type window struct {
	mu sync.Mutex

	hudActive bool
	dashX     int
	dashY     int
	dashW     int
	dashH     int
}

// The dashboard's own size limits, restored when the overlay closes — they
// mirror the MinWidth/MinHeight given in main.go. The overlay replaces them
// with a pin at its own size.
const (
	dashMinW = 220
	dashMinH = 140
)

// minHudH is the shortest the overlay may be: a measurement taken while the
// frontend is mid-layout must not be able to collapse the window to nothing.
const minHudH = 90

// SetHudMode switches the window between the dashboard and the compact
// always-on-top overlay.
func (a *App) SetHudMode(on bool) {
	a.win.mu.Lock()
	defer a.win.mu.Unlock()

	if on == a.win.hudActive {
		return
	}
	if on {
		a.enterHud()
	} else {
		a.leaveHud()
	}
	a.win.hudActive = on
}

// ToggleHud flips between the dashboard and the overlay. It exists for the
// system-wide hotkey, which fires while a game holds focus and therefore
// cannot go through the frontend — so this tells the frontend afterwards
// instead, via the "hud" event.
func (a *App) ToggleHud() {
	a.win.mu.Lock()
	on := !a.win.hudActive
	a.win.mu.Unlock()

	a.SetHudMode(on)
	runtime.EventsEmit(a.ctx, "hud", on)
}

// ResetFPSStats restarts the frame-rate average and lows, for the hotkey that
// marks the start of a run worth measuring.
func (a *App) ResetFPSStats() {
	if a.collector != nil {
		a.collector.ResetFPS()
	}
}

func (a *App) enterHud() {
	a.win.dashX, a.win.dashY = runtime.WindowGetPosition(a.ctx)
	a.win.dashW, a.win.dashH = runtime.WindowGetSize(a.ctx)

	// First, before anything raises or moves the window: from here on none of
	// it can take the foreground away from the game underneath.
	applyOverlayStyles(true)

	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	// The saved height only avoids a visible jump: the frontend measures its
	// own contents as soon as it renders and calls FitHudSize with the real one.
	// Clamped all the same — a height saved on a larger monitor would otherwise
	// open the overlay hanging off the bottom of this one.
	a.cfg.Hud.H = clampHudHeight(a.cfg.Hud.H)
	a.pinHudSize(a.cfg.Hud.W, a.cfg.Hud.H)

	x, y := hudPosition(a.cfg.Hud)
	moveWindowTo(a.ctx, x, y)

	if a.cfg.Hud.ClickThrough {
		applyClickThrough(true)
	}

	// Wails' always-on-top is a one-shot flag, which a borderless-fullscreen
	// game overrides when it takes focus. The same loop holds an anchored
	// overlay in its corner. See window_windows.go.
	startTopmostKeeper(a.hudAnchorPos)
}

// hudAnchorPos is where a corner-anchored overlay belongs right now, recomputed
// each time because the work area it is measured against can change underneath
// it. A free-placed overlay reports false: that one is the user's to position.
func (a *App) hudAnchorPos() (int, int, bool) {
	a.win.mu.Lock()
	defer a.win.mu.Unlock()

	if !a.win.hudActive || !anchored(a.cfg.Hud.Anchor) {
		return 0, 0, false
	}
	x, y := AnchorPosition(a.cfg.Hud.Anchor, a.cfg.Hud.W, a.cfg.Hud.H)
	return x, y, true
}

func (a *App) leaveHud() {
	stopTopmostKeeper()
	applyClickThrough(false)  // the dashboard must always take the mouse
	applyOverlayStyles(false) // ...and must be able to take the foreground

	if !anchored(a.cfg.Hud.Anchor) {
		x, y := runtime.WindowGetPosition(a.ctx)
		// Never persist a position that would hide the overlay next time.
		a.cfg.Hud.X, a.cfg.Hud.Y = ClampToScreen(x, y, a.cfg.Hud.W, a.cfg.Hud.H)
	}
	config.Save(a.cfg)

	// Hand the dashboard its limits back before restoring its geometry: the
	// overlay leaves the window pinned to overlay size, and a window cannot
	// grow past a maximum that is still in force.
	unpinSize(a.ctx)
	runtime.WindowSetMinSize(a.ctx, dashMinW, dashMinH)

	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	if a.win.dashW > 0 {
		runtime.WindowSetSize(a.ctx, a.win.dashW, a.win.dashH)
		moveWindowTo(a.ctx, a.win.dashX, a.win.dashY)
	}
}

// FitHudSize sizes the overlay window around its contents. The HUD draws a
// variable number of rows and graphs — how many is up to the user's metric
// selection — and since the window can no longer be resized by hand, the
// frontend has to say how tall it needs to be.
//
// It reports the height its content wants together with the viewport it is
// currently given; the difference between that viewport and the window is the
// frame Windows keeps around it, which only this side can measure.
func (a *App) FitHudSize(contentH, viewportH int) {
	a.win.mu.Lock()
	defer a.win.mu.Unlock()

	if !a.win.hudActive || contentH <= 0 || viewportH <= 0 {
		return
	}

	_, winH := runtime.WindowGetSize(a.ctx)
	h := clampHudHeight(contentH + winH - viewportH)
	if h == a.cfg.Hud.H {
		return
	}
	a.cfg.Hud.H = h
	a.pinHudSize(a.cfg.Hud.W, h)

	// A bottom-anchored overlay grows upwards from its corner: left alone, it
	// would drift down the screen by exactly the height it just gained.
	if anchored(a.cfg.Hud.Anchor) {
		x, y := AnchorPosition(a.cfg.Hud.Anchor, a.cfg.Hud.W, h)
		moveWindowTo(a.ctx, x, y)
	}
	config.Save(a.cfg)
}

// pinHudSize resizes the window and holds it at that exact size.
//
// A frameless window still carries Windows' sizing border, and there is no
// run-time flag that takes it away — but a window whose minimum and maximum
// track sizes are identical cannot be resized, which from the outside is the
// same thing. The overlay is sized by its contents, so the drag handles could
// only ever clip it or leave a transparent margin.
func (a *App) pinHudSize(w, h int) {
	unpinSize(a.ctx) // a window cannot shrink below a minimum still in force
	runtime.WindowSetSize(a.ctx, w, h)
	runtime.WindowSetMinSize(a.ctx, w, h)
	runtime.WindowSetMaxSize(a.ctx, w, h)
}

func unpinSize(ctx context.Context) {
	runtime.WindowSetMaxSize(ctx, 0, 0)
	runtime.WindowSetMinSize(ctx, 0, 0)
}

func clampHudHeight(h int) int {
	if h < minHudH {
		return minHudH
	}
	if maxH := MaxOverlayHeight(); h > maxH {
		return maxH
	}
	return h
}

// anchored reports whether the overlay is pinned to a corner rather than left
// where the user dragged it. Empty is how a config written before the setting
// existed reads, and means free.
func anchored(anchor string) bool { return anchor != "free" && anchor != "" }

// SetClickThrough switches whether the overlay ignores the mouse. Persisted,
// applied immediately while the HUD is up, and announced via the
// "clickthrough" event so the HUD indicator and Settings stay in step.
func (a *App) SetClickThrough(on bool) {
	a.win.mu.Lock()
	a.cfg.Hud.ClickThrough = on
	if a.win.hudActive {
		applyClickThrough(on)
	}
	a.win.mu.Unlock()

	config.Save(a.cfg)
	runtime.EventsEmit(a.ctx, "clickthrough", on)
}

// ToggleClickThrough exists for the hotkey: a click-through overlay cannot be
// clicked back to normal, so the keyboard has to be able to do it.
func (a *App) ToggleClickThrough() {
	a.SetClickThrough(!a.cfg.Hud.ClickThrough)
}

// hudPosition resolves where the overlay should appear: a fixed corner of the
// primary monitor, or the last free-drag position — clamped, so a position
// saved on a monitor that has since been unplugged cannot park the overlay
// where nobody can see it.
func hudPosition(hud config.HudConfig) (int, int) {
	if !anchored(hud.Anchor) {
		return ClampToScreen(hud.X, hud.Y, hud.W, hud.H)
	}
	return AnchorPosition(hud.Anchor, hud.W, hud.H)
}
