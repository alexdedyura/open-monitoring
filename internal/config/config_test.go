package config

import (
	"slices"
	"testing"
)

// These tests exercise clamp and migrate directly rather than going through
// Load and Save, which read and write the real user config file.

// A hand-edited or downgraded file must not be able to put the app into a state
// the UI cannot represent.
func TestClampForcesValuesIntoRange(t *testing.T) {
	cfg := Config{
		SampleIntervalMs: 10,
		MaxRecordMinutes: 9000,
		Theme:            "solarized",
		UiScale:          3.7,
		Hud:              HudConfig{Opacity: 5, Anchor: "middle"},
	}
	Clamp(&cfg)

	if cfg.SampleIntervalMs != 250 {
		t.Errorf("SampleIntervalMs = %d, want the 250 floor", cfg.SampleIntervalMs)
	}
	if cfg.MaxRecordMinutes != 240 {
		t.Errorf("MaxRecordMinutes = %d, want the 240 cap", cfg.MaxRecordMinutes)
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %q, want the dark fallback", cfg.Theme)
	}
	if cfg.UiScale != 0 {
		t.Errorf("UiScale = %v, want 0 (match Windows)", cfg.UiScale)
	}
	if cfg.Hud.Opacity != 0.85 {
		t.Errorf("Hud.Opacity = %v, want the 0.85 default", cfg.Hud.Opacity)
	}
	if cfg.Hud.Anchor != "free" {
		t.Errorf("Hud.Anchor = %q, want the free fallback", cfg.Hud.Anchor)
	}
}

// Valid settings must survive clamping untouched, or saving would quietly
// rewrite the user's choices.
func TestClampLeavesValidValuesAlone(t *testing.T) {
	cfg := Default()
	cfg.Theme = "light"
	cfg.UiScale = 1.25
	cfg.SampleIntervalMs = 500
	cfg.Hud.Anchor = "br"

	want := cfg
	Clamp(&cfg)

	if cfg.Theme != want.Theme || cfg.UiScale != want.UiScale ||
		cfg.SampleIntervalMs != want.SampleIntervalMs || cfg.Hud.Anchor != want.Hud.Anchor {
		t.Errorf("clamp modified a valid config:\n got %+v\nwant %+v", cfg, want)
	}
}

// A pre-v3 file refers to HUD rows that no longer exist, so the selection has
// to be reset rather than carried forward as a set of dead keys.
func TestMigrateResetsPreV3HudRows(t *testing.T) {
	cfg := Default()
	cfg.Hud.Metrics = []string{"someOldKey", "anotherOldKey"}

	migrate(&cfg, 2)

	if slices.Contains(cfg.Hud.Metrics, "someOldKey") {
		t.Error("pre-v3 HUD rows were carried forward")
	}
	if len(cfg.Hud.Metrics) == 0 {
		t.Error("HUD rows were cleared instead of reset to defaults")
	}
}

// v5 introduced the sparklines and automatic UI scaling. Installs pinned at
// 100% were only there because that used to be the default.
func TestMigrateV5(t *testing.T) {
	cfg := Default()
	cfg.UiScale = 1
	cfg.Hud.H = 400
	cfg.Hud.Metrics = []string{"cpu", "gpu"}

	migrate(&cfg, 4)

	if cfg.UiScale != 0 {
		t.Errorf("UiScale = %v, want 0 so the app follows Windows", cfg.UiScale)
	}
	for _, key := range []string{"frameGraph", "fpsGraph"} {
		if !slices.Contains(cfg.Hud.Metrics, key) {
			t.Errorf("migration did not enable %q", key)
		}
	}
	if cfg.Hud.H < 620 {
		t.Errorf("Hud.H = %d, too short for the graphs", cfg.Hud.H)
	}
	// Rows the user had chosen must survive.
	if !slices.Contains(cfg.Hud.Metrics, "cpu") {
		t.Error("migration dropped a row the user had enabled")
	}
}

// A deliberately chosen scale must not be reset by the v5 migration.
func TestMigrateV5KeepsExplicitScale(t *testing.T) {
	cfg := Default()
	cfg.UiScale = 1.5

	migrate(&cfg, 4)

	if cfg.UiScale != 1.5 {
		t.Errorf("UiScale = %v, want the user's 1.5 kept", cfg.UiScale)
	}
}

// Migrations must be safe to apply to anything older, since users skip
// releases: a v0 file has to end up equivalent to a current one.
func TestMigrateFromScratchMatchesDefaults(t *testing.T) {
	cfg := Default()
	cfg.Hud.Metrics = nil

	migrate(&cfg, 0)
	Clamp(&cfg)

	if len(cfg.Hud.Metrics) != len(defaultHudMetrics()) {
		t.Errorf("migrating from v0 gave %d HUD rows, want the %d defaults",
			len(cfg.Hud.Metrics), len(defaultHudMetrics()))
	}
}

// A current file must pass through untouched, or every launch would rewrite it.
func TestMigrateIsNoOpAtCurrentVersion(t *testing.T) {
	cfg := Default()
	cfg.UiScale = 1 // a valid explicit choice at v6
	want := cfg

	migrate(&cfg, currentVersion)

	if cfg.UiScale != want.UiScale {
		t.Errorf("UiScale = %v, want %v untouched at the current version", cfg.UiScale, want.UiScale)
	}
	if !slices.Equal(cfg.Hud.Metrics, want.Hud.Metrics) {
		t.Error("HUD rows were rewritten at the current version")
	}
}
