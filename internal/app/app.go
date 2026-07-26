// Package app is the desktop application itself: it owns the metric collector,
// the session store, the recording state machine, and the single window that
// toggles between the dashboard and the HUD overlay.
//
// Every exported method on App is callable from the frontend — Wails generates
// JavaScript bindings for them — so the exported surface here is effectively
// the app's API. See bindings.go for it.
package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/config"
	"open-monitoring/internal/hotkey"
	"open-monitoring/internal/metrics"
	"open-monitoring/internal/store"
	"open-monitoring/internal/stress"
)

// App holds everything with a lifetime longer than a single call.
type App struct {
	ctx context.Context

	version   string // from wails.json, embedded by main
	cfg       config.Config
	collector *metrics.Collector
	store     *store.Store
	stress    *stress.Runner

	// Threshold-alert state, keyed by rule id. See alerts.go.
	alertMu sync.Mutex
	alerts  map[string]*alertState

	// The release found by the last update check, kept for ApplyUpdate.
	updMu      sync.Mutex
	updRelease *selfupdate.Release

	// The expensive half of the machine description never changes, so it is
	// gathered once. Wails does not promise that binding calls are serialised,
	// hence the Once rather than a plain nil check.
	staticOnce sync.Once
	staticInfo metrics.StaticInfo

	recMu sync.Mutex
	rec   *recording

	win window // dashboard/HUD geometry, guarded by its own mutex

	// The shortcuts can be re-registered from a binding call while Startup is
	// still finishing, and Wails does not promise binding calls are serialised.
	hotkeyMu   sync.Mutex
	hotkeys    *hotkey.Manager
	hotkeyErrs []error // one per binding below, nil where the shortcut is live
}

// The order the shortcuts are registered in. GetHotkeyStatus reports them by
// the same index, so the two lists have to stay in step.
const (
	hotkeyToggle = iota
	hotkeyReset
	hotkeyClickThrough
)

// New builds the app. The stress runner is created here rather than in Startup
// because the frontend may ask for its status before the window is ready.
func New(version string) *App {
	a := &App{version: version, alerts: map[string]*alertState{}}
	a.stress = stress.NewRunner(a.emitStress)
	return a
}

// Startup is wired to Wails' OnStartup. A failure to open the session database
// is not fatal: the app still monitors, it just cannot record.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.cfg = config.Load()

	st, err := store.Open(filepath.Join(config.Dir(), "sessions.db"))
	if err != nil {
		runtime.LogErrorf(ctx, "session store unavailable: %v", err)
	}
	a.store = st

	// Retention is opt-in (0 keeps everything) and only ever touches finished
	// recordings.
	if st != nil && a.cfg.KeepSessionsDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -a.cfg.KeepSessionsDays).UnixMilli()
		if err := st.DeleteSessionsBefore(cutoff); err != nil {
			runtime.LogErrorf(ctx, "session retention: %v", err)
		}
	}

	a.collector = metrics.NewCollector(a.cfg.SampleIntervalMs, a.onSample, a.onFPS)
	a.collector.Start()

	a.registerHotkeys()
	go a.watchUpdates()
}

// registerHotkeys claims the two system-wide shortcuts. A combination another
// application already owns cannot be claimed, and that is not worth failing
// startup over — the app runs without that one and Settings reports it, which
// is the only way the user can tell a dead shortcut from a broken feature.
func (a *App) registerHotkeys() {
	keys, errs := hotkey.Register([]hotkey.Binding{
		hotkeyToggle:       {Combo: a.cfg.Hud.HotkeyToggle, Do: a.ToggleHud},
		hotkeyReset:        {Combo: a.cfg.Hud.HotkeyReset, Do: a.ResetFPSStats},
		hotkeyClickThrough: {Combo: a.cfg.Hud.HotkeyClickThrough, Do: a.ToggleClickThrough},
	})
	for _, err := range errs {
		if err != nil {
			runtime.LogWarningf(a.ctx, "hotkey not registered: %v", err)
		}
	}

	a.hotkeyMu.Lock()
	a.hotkeys, a.hotkeyErrs = keys, errs
	a.hotkeyMu.Unlock()
}

// reregisterHotkeys swaps in the combinations the config now holds. A
// registration belongs to the listener thread that made it, so changing one
// means stopping that thread and starting a fresh one — there is no Win32 call
// to amend a registration in place.
func (a *App) reregisterHotkeys() {
	a.hotkeyMu.Lock()
	previous := a.hotkeys
	a.hotkeyMu.Unlock()

	previous.Stop() // releases the combinations before the new ones ask for them
	a.registerHotkeys()
}

// Shutdown is wired to Wails' OnShutdown. Stopping the recording first flushes
// whatever is still buffered, and stopping the stress runner gives the disk job
// time to delete its multi-gigabyte test file.
func (a *App) Shutdown(ctx context.Context) {
	a.StopRecording()

	a.hotkeyMu.Lock()
	keys := a.hotkeys
	a.hotkeyMu.Unlock()
	keys.Stop()

	if a.stress != nil {
		a.stress.StopAll()
	}
	if a.collector != nil {
		a.collector.Stop()
	}
	if a.store != nil {
		a.store.Close()
	}
}

// onSample runs on the collector's goroutine for every new sample: it pushes
// the sample to the frontend, checks it against the alert thresholds and,
// while recording, into the session.
func (a *App) onSample(s metrics.Sample) {
	runtime.EventsEmit(a.ctx, "sample", s)
	a.checkAlerts(s)
	a.record(s)
}

// onFPS carries frame-rate metrics on their own cadence, several times per
// sample, so the HUD readout and its graphs keep up with the game. Recording
// stays on the sample stream: a session is a fixed-interval series.
func (a *App) onFPS(m *metrics.FPSMetrics) {
	runtime.EventsEmit(a.ctx, "fps", m)
}
