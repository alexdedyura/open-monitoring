package app

import (
	"open-monitoring/internal/config"
	"open-monitoring/internal/metrics"
	"open-monitoring/internal/store"
)

// This file collects the plain request/response methods the frontend calls.
// Recording, CSV export and window control live in recording.go and window.go
// because they carry real logic; everything here is a thin accessor.

// GetStaticInfo returns the machine description. The expensive parts (WMI and
// gopsutil queries) are gathered once and cached; source status and everything
// the sensor helper supplies are refreshed on every call, because the helper
// may still have been starting up when the UI first asked.
func (a *App) GetStaticInfo() metrics.StaticInfo {
	a.staticOnce.Do(func() { a.staticInfo = a.collector.Static() })

	info := a.staticInfo
	info.SourceStatus = a.collector.Status()
	info.ApplyBridgeInfo(a.collector.BridgeInfo())
	return info
}

func (a *App) GetConfig() config.Config {
	return a.cfg
}

// SaveConfig validates and persists the settings, applying the ones that take
// effect immediately.
func (a *App) SaveConfig(cfg config.Config) error {
	config.Clamp(&cfg)

	rebind := cfg.Hud.HotkeyToggle != a.cfg.Hud.HotkeyToggle ||
		cfg.Hud.HotkeyReset != a.cfg.Hud.HotkeyReset

	a.cfg = cfg
	a.collector.SetInterval(cfg.SampleIntervalMs)
	if rebind {
		// Before Save, so that GetHotkeyStatus right after this call already
		// describes the combinations that were just stored.
		a.reregisterHotkeys()
	}
	return config.Save(cfg)
}

// HotkeyStatus says whether each shortcut is actually live. A combination can
// be spelled perfectly and still never fire, because another application
// registered it first and Windows will not say which one — without this the
// user has no way to tell that from a broken feature.
type HotkeyStatus struct {
	Toggle bool `json:"toggle"`
	Reset  bool `json:"reset"`
}

func (a *App) GetHotkeyStatus() HotkeyStatus {
	a.hotkeyMu.Lock()
	defer a.hotkeyMu.Unlock()

	live := func(i int) bool { return i < len(a.hotkeyErrs) && a.hotkeyErrs[i] == nil }
	return HotkeyStatus{Toggle: live(hotkeyToggle), Reset: live(hotkeyReset)}
}

// GetHistory returns buffered samples covering the last `seconds` seconds, so
// the charts open already populated instead of drawing in from empty.
func (a *App) GetHistory(seconds int) []metrics.Sample {
	return a.collector.History(seconds)
}

// GetDiskHealth returns the physical drives with their identity and SMART
// wear data. Served from a cache the collector refreshes in the background.
func (a *App) GetDiskHealth() []metrics.DiskHealthView {
	return a.collector.DiskHealth()
}

func (a *App) ListSessions() ([]store.SessionInfo, error) {
	if a.store == nil {
		return nil, errNoStore
	}
	return a.store.ListSessions()
}

func (a *App) GetSessionSamples(id int64) ([]metrics.Sample, error) {
	if a.store == nil {
		return nil, errNoStore
	}
	return a.store.SessionSamples(id)
}

func (a *App) DeleteSession(id int64) error {
	if a.store == nil {
		return errNoStore
	}
	return a.store.DeleteSession(id)
}
