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

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/config"
	"open-monitoring/internal/metrics"
	"open-monitoring/internal/store"
)

// App holds everything with a lifetime longer than a single call.
type App struct {
	ctx context.Context

	cfg       config.Config
	collector *metrics.Collector
	store     *store.Store

	// The expensive half of the machine description never changes, so it is
	// gathered once. Wails does not promise that binding calls are serialised,
	// hence the Once rather than a plain nil check.
	staticOnce sync.Once
	staticInfo metrics.StaticInfo

	recMu sync.Mutex
	rec   *recording

	win window // dashboard/HUD geometry, guarded by its own mutex
}

func New() *App { return &App{} }

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

	a.collector = metrics.NewCollector(a.cfg.SampleIntervalMs, a.onSample)
	a.collector.Start()
}

// Shutdown is wired to Wails' OnShutdown. Stopping the recording first flushes
// whatever is still buffered.
func (a *App) Shutdown(ctx context.Context) {
	a.StopRecording()
	if a.collector != nil {
		a.collector.Stop()
	}
	if a.store != nil {
		a.store.Close()
	}
}

// onSample runs on the collector's goroutine for every new sample: it pushes
// the sample to the frontend and, while recording, into the session.
func (a *App) onSample(s metrics.Sample) {
	runtime.EventsEmit(a.ctx, "sample", s)
	a.record(s)
}
