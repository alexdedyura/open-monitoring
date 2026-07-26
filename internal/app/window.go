package app

import (
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

	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetSize(a.ctx, a.cfg.Hud.W, a.cfg.Hud.H)

	x, y := hudPosition(a.cfg.Hud)
	runtime.WindowSetPosition(a.ctx, x, y)

	if a.cfg.Hud.ClickThrough {
		applyClickThrough(true)
	}

	// Wails' always-on-top is a one-shot flag, which a borderless-fullscreen
	// game overrides when it takes focus. See window_windows.go.
	startTopmostKeeper()
}

func (a *App) leaveHud() {
	stopTopmostKeeper()
	applyClickThrough(false) // the dashboard must always take the mouse

	a.cfg.Hud.W, a.cfg.Hud.H = runtime.WindowGetSize(a.ctx)
	if a.cfg.Hud.Anchor == "free" {
		x, y := runtime.WindowGetPosition(a.ctx)
		// Never persist a position that would hide the overlay next time.
		a.cfg.Hud.X, a.cfg.Hud.Y = ClampToScreen(x, y, a.cfg.Hud.W, a.cfg.Hud.H)
	}
	config.Save(a.cfg)

	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	if a.win.dashW > 0 {
		runtime.WindowSetSize(a.ctx, a.win.dashW, a.win.dashH)
		runtime.WindowSetPosition(a.ctx, a.win.dashX, a.win.dashY)
	}
}

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
	if hud.Anchor == "free" || hud.Anchor == "" {
		return ClampToScreen(hud.X, hud.Y, hud.W, hud.H)
	}
	return AnchorPosition(hud.Anchor, hud.W, hud.H)
}
