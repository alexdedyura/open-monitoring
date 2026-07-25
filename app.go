package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/config"
	"open-monitoring/internal/metrics"
	"open-monitoring/internal/store"
)

const flushEvery = 10 // samples per SQLite batch

type App struct {
	ctx context.Context
	cfg config.Config
	col *metrics.Collector
	st  *store.Store

	staticInfo     *metrics.StaticInfo
	restartPending bool

	recMu sync.Mutex
	rec   *recState

	winMu                      sync.Mutex
	hud                        bool
	dashX, dashY, dashW, dashH int
}

type recState struct {
	sessionID int64
	name      string
	startedAt int64
	count     int
	buf       []metrics.Sample
}

// RecStatus is what the frontend sees about the current recording.
type RecStatus struct {
	Active     bool   `json:"active"`
	SessionID  int64  `json:"sessionId"`
	Name       string `json:"name"`
	StartedAt  int64  `json:"startedAt"`
	Count      int    `json:"count"`
	MaxMinutes int    `json:"maxMinutes"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cfg = config.Load()

	st, err := store.Open(filepath.Join(config.Dir(), "sessions.db"))
	if err != nil {
		runtime.LogErrorf(ctx, "store: %v", err)
	}
	a.st = st

	if !a.cfg.EnableCpuSensors {
		// Leave no WinRing0 service registered while the feature is off.
		go metrics.CleanupRing0Driver()
	}

	a.col = metrics.NewCollector(metrics.Options{
		IntervalMs:       a.cfg.SampleIntervalMs,
		LHMUrl:           a.cfg.LHMUrl,
		EnableCpuSensors: a.cfg.EnableCpuSensors,
	}, a.onSample)
	a.col.Start()
}

func (a *App) shutdown(ctx context.Context) {
	a.StopRecording()
	if a.col != nil {
		a.col.Stop()
	}
	if a.st != nil {
		a.st.Close()
	}
}

func (a *App) onSample(s metrics.Sample) {
	runtime.EventsEmit(a.ctx, "sample", s)

	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.rec == nil {
		return
	}
	a.rec.buf = append(a.rec.buf, s)
	a.rec.count++
	if len(a.rec.buf) >= flushEvery {
		a.flushLocked()
	}
	if s.T-a.rec.startedAt >= int64(a.cfg.MaxRecordMinutes)*60_000 {
		a.stopLocked() // auto-stop at the configured cap (up to 4h)
		runtime.EventsEmit(a.ctx, "recording", a.recStatusLocked())
	}
}

func (a *App) flushLocked() {
	if a.st == nil || a.rec == nil || len(a.rec.buf) == 0 {
		return
	}
	if err := a.st.AppendSamples(a.rec.sessionID, a.rec.buf); err != nil {
		runtime.LogErrorf(a.ctx, "append samples: %v", err)
	}
	a.rec.buf = a.rec.buf[:0]
}

func (a *App) stopLocked() {
	if a.rec == nil {
		return
	}
	a.flushLocked()
	if a.st != nil {
		a.st.EndSession(a.rec.sessionID, time.Now().UnixMilli())
	}
	a.rec = nil
}

func (a *App) recStatusLocked() RecStatus {
	st := RecStatus{MaxMinutes: a.cfg.MaxRecordMinutes}
	if a.rec != nil {
		st.Active = true
		st.SessionID = a.rec.sessionID
		st.Name = a.rec.name
		st.StartedAt = a.rec.startedAt
		st.Count = a.rec.count
	}
	return st
}

// ---- bound methods ----

func (a *App) GetStaticInfo() metrics.StaticInfo {
	if a.staticInfo == nil {
		info := a.col.Static()
		a.staticInfo = &info
	}
	info := *a.staticInfo
	info.LHMConnected = a.col.LHMAlive()
	return info
}

func (a *App) GetConfig() config.Config {
	return a.cfg
}

func (a *App) SaveConfig(cfg config.Config) error {
	if cfg.SampleIntervalMs < 250 {
		cfg.SampleIntervalMs = 250
	}
	if cfg.MaxRecordMinutes < 1 || cfg.MaxRecordMinutes > 240 {
		cfg.MaxRecordMinutes = 240
	}
	// Starting or stopping the kernel-driver-backed sensor source mid-session
	// is not worth the complexity; it takes effect on the next launch.
	a.restartPending = cfg.EnableCpuSensors != a.cfg.EnableCpuSensors
	a.cfg = cfg
	a.col.SetInterval(cfg.SampleIntervalMs)
	return config.Save(cfg)
}

// RestartPending reports whether a setting was changed that only applies after
// the app is restarted.
func (a *App) RestartPending() bool {
	return a.restartPending
}

func (a *App) GetHistory(seconds int) []metrics.Sample {
	return a.col.History(seconds)
}

// GetDiskHealth returns physical drives with WMI status and, when the LHM
// bridge runs elevated, SMART temperature / remaining life.
func (a *App) GetDiskHealth() []metrics.DiskHealthView {
	return a.col.DiskHealth()
}

func (a *App) StartRecording(name string) (RecStatus, error) {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.rec != nil {
		return a.recStatusLocked(), nil
	}
	if a.st == nil {
		return RecStatus{}, fmt.Errorf("storage unavailable")
	}
	if name == "" {
		name = "Session " + time.Now().Format("2006-01-02 15:04")
	}
	now := time.Now().UnixMilli()
	id, err := a.st.CreateSession(name, now, a.cfg.SampleIntervalMs)
	if err != nil {
		return RecStatus{}, err
	}
	a.rec = &recState{sessionID: id, name: name, startedAt: now}
	st := a.recStatusLocked()
	runtime.EventsEmit(a.ctx, "recording", st)
	return st, nil
}

func (a *App) StopRecording() RecStatus {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	a.stopLocked()
	st := a.recStatusLocked()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "recording", st)
	}
	return st
}

func (a *App) GetRecStatus() RecStatus {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	return a.recStatusLocked()
}

func (a *App) ListSessions() ([]store.SessionInfo, error) {
	if a.st == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	return a.st.ListSessions()
}

func (a *App) GetSessionSamples(id int64) ([]metrics.Sample, error) {
	if a.st == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	return a.st.SessionSamples(id)
}

func (a *App) DeleteSession(id int64) error {
	if a.st == nil {
		return fmt.Errorf("storage unavailable")
	}
	return a.st.DeleteSession(id)
}

// ExportSessionCSV asks for a target path and writes the session as CSV.
// Returns the chosen path ("" if the user cancelled).
func (a *App) ExportSessionCSV(id int64) (string, error) {
	if a.st == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	name, err := a.st.SessionName(id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: sanitize(name) + ".csv",
		Filters:         []runtime.FileFilter{{DisplayName: "CSV", Pattern: "*.csv"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	samples, err := a.st.SessionSamples(id)
	if err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{
		"time", "cpu_usage_pct", "cpu_temp_c", "cpu_power_w", "cpu_clock_mhz",
		"ram_used_mb", "ram_used_pct",
		"gpu_usage_pct", "gpu_temp_c", "vram_used_mb", "gpu_power_w", "gpu_core_mhz", "gpu_fan_pct",
		"disk_read_kbps", "disk_write_kbps", "net_up_kbps", "net_down_kbps",
		"fps_cur", "fps_avg", "fps_low1", "fps_low01",
	})
	for _, s := range samples {
		var dr, dw float64
		for _, d := range s.Disks {
			dr += d.ReadBps
			dw += d.WriteBps
		}
		g := s.GPU
		if g == nil {
			g = &metrics.GPUMetrics{}
		}
		fps := s.FPS
		if fps == nil {
			fps = &metrics.FPSMetrics{}
		}
		w.Write([]string{
			time.UnixMilli(s.T).Format("2006-01-02 15:04:05"),
			ff(s.CPU.Usage), ff(s.CPU.TempC), ff(s.CPU.PowerW), ff(s.CPU.ClockMHz),
			ff(float64(s.Mem.Used) / 1024 / 1024), ff(s.Mem.UsedPercent),
			ff(g.Usage), ff(g.TempC), ff(g.MemUsedMB), ff(g.PowerW), ff(g.CoreMHz), ff(g.FanPercent),
			ff(dr / 1024), ff(dw / 1024), ff(s.Net.UpBps / 1024), ff(s.Net.DownBps / 1024),
			ff(fps.Cur), ff(fps.Avg), ff(fps.Low1), ff(fps.Low01),
		})
	}
	return path, nil
}

// SetHudMode switches the single window between the dashboard and a compact
// always-on-top overlay, remembering each mode's geometry.
func (a *App) SetHudMode(on bool) {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	if on == a.hud {
		return
	}
	if on {
		a.dashX, a.dashY = runtime.WindowGetPosition(a.ctx)
		a.dashW, a.dashH = runtime.WindowGetSize(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetSize(a.ctx, a.cfg.Hud.W, a.cfg.Hud.H)
		x, y := a.hudPosition()
		runtime.WindowSetPosition(a.ctx, x, y)
		hudTopmostOn() // keep re-asserting topmost over borderless games
	} else {
		hudTopmostOff()
		if a.cfg.Hud.Anchor == "free" {
			a.cfg.Hud.X, a.cfg.Hud.Y = runtime.WindowGetPosition(a.ctx)
		}
		a.cfg.Hud.W, a.cfg.Hud.H = runtime.WindowGetSize(a.ctx)
		config.Save(a.cfg)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
		if a.dashW > 0 {
			runtime.WindowSetSize(a.ctx, a.dashW, a.dashH)
			runtime.WindowSetPosition(a.ctx, a.dashX, a.dashY)
		}
	}
	a.hud = on
}

// hudPosition resolves the overlay position: a fixed screen corner (with a
// small margin) or the last free-drag position.
func (a *App) hudPosition() (int, int) {
	const margin = 16
	if a.cfg.Hud.Anchor == "free" || a.cfg.Hud.Anchor == "" {
		return a.cfg.Hud.X, a.cfg.Hud.Y
	}
	sw, sh := 1920, 1080
	if screens, err := runtime.ScreenGetAll(a.ctx); err == nil {
		for _, s := range screens {
			if s.IsCurrent || s.IsPrimary {
				sw, sh = s.Size.Width, s.Size.Height
				if s.IsCurrent {
					break
				}
			}
		}
	}
	x, y := margin, margin
	if a.cfg.Hud.Anchor == "tr" || a.cfg.Hud.Anchor == "br" {
		x = sw - a.cfg.Hud.W - margin
	}
	if a.cfg.Hud.Anchor == "bl" || a.cfg.Hud.Anchor == "br" {
		y = sh - a.cfg.Hud.H - margin
	}
	return x, y
}

func ff(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func sanitize(s string) string {
	out := []rune{}
	for _, r := range s {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			out = append(out, '_')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
