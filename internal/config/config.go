// Package config stores user settings as JSON in the app's data directory.
//
// Settings are small, read once at startup and written on every change, so the
// whole file is loaded and saved as a unit. Load never fails: a missing or
// corrupt file falls back to defaults, because refusing to start over an
// unreadable preferences file would be worse than losing the preferences.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// currentVersion is bumped whenever a stored file needs fixing up on load.
// See migrate for what each step does.
const currentVersion = 8

type Config struct {
	Version          int     `json:"version"`
	SampleIntervalMs int     `json:"sampleIntervalMs"`
	MaxRecordMinutes int     `json:"maxRecordMinutes"`
	Theme            string  `json:"theme"`   // dark | light
	UiScale          float64 `json:"uiScale"` // 0 = follow the Windows display scaling

	Hud HudConfig `json:"hud"`
}

type HudConfig struct {
	Metrics          []string `json:"metrics"` // enabled row keys, grouped into sections by the UI
	Opacity          float64  `json:"opacity"` // 0.2 .. 1
	Anchor           string   `json:"anchor"`  // free | tl | tr | bl | br
	X                int      `json:"x"`
	Y                int      `json:"y"`
	W                int      `json:"w"`
	H                int      `json:"h"`
	FsAlertDismissed bool     `json:"fsAlertDismissed"` // "exclusive fullscreen hides the HUD" notice
}

// defaultHudMetrics mirrors a classic in-game OSD: a GPU block, a CPU block,
// memory, and frame rate, each rendered as a titled section.
func defaultHudMetrics() []string {
	return []string{
		"gpu", "gpuTemp", "gpuClock", "gpuMemClock", "gpuPow", "vram",
		"cpu", "cpuTemp", "cpuClock", "cpuPow",
		"ram", "ramLoad", "ramSpeed",
		"fps", "fpsAvg", "fpsLow1", "fpsLow01", "frameGraph", "fpsGraph",
	}
}

func Default() Config {
	return Config{
		Version:          currentVersion,
		SampleIntervalMs: 1000,
		MaxRecordMinutes: 240,
		Theme:            "dark",
		UiScale:          0, // match Windows
		Hud: HudConfig{
			Metrics: defaultHudMetrics(),
			Opacity: 0.85,
			Anchor:  "free",
			X:       40,
			Y:       40,
			W:       320,
			H:       620,
		},
	}
}

// Dir is the app's data directory, created if missing. Config and the session
// database live side by side there.
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	dir := filepath.Join(base, "OpenMonitoring")
	os.MkdirAll(dir, 0o755)
	return dir
}

func path() string { return filepath.Join(Dir(), "config.json") }

// Load reads the stored settings, falling back to defaults for anything
// missing, out of range, or unreadable.
func Load() Config {
	cfg := Default()

	data, err := os.ReadFile(path())
	if err != nil {
		return cfg // first run
	}

	// Unmarshalling over the defaults means absent keys keep their default.
	// Version has to start at zero for that same reason: a file written before
	// the field existed must read as v0 so its migrations still run.
	cfg.Version = 0
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default() // corrupt; better to reset than to start half-configured
	}

	stored := cfg.Version
	Clamp(&cfg)
	migrate(&cfg, stored)

	if stored < currentVersion {
		cfg.Version = currentVersion
		Save(cfg)
	}
	return cfg
}

// Clamp forces every value back into its supported range. It guards the two
// ways an invalid value can arrive: a hand-edited or downgraded file on load,
// and a frontend save.
func Clamp(cfg *Config) {
	if cfg.SampleIntervalMs < 250 {
		cfg.SampleIntervalMs = 250
	}
	if cfg.MaxRecordMinutes < 1 || cfg.MaxRecordMinutes > 240 {
		cfg.MaxRecordMinutes = 240
	}
	if cfg.Hud.Opacity < 0.2 || cfg.Hud.Opacity > 1 {
		cfg.Hud.Opacity = 0.85
	}
	if cfg.Theme != "light" {
		cfg.Theme = "dark"
	}
	switch cfg.UiScale {
	case 0, 1, 1.25, 1.5, 2:
	default:
		cfg.UiScale = 0
	}
	switch cfg.Hud.Anchor {
	case "free", "tl", "tr", "bl", "br":
	default:
		cfg.Hud.Anchor = "free"
	}
}

// migrate fixes up a file written by an older version. Each step is written to
// be safe to apply to anything older than it, since a user may skip releases.
func migrate(cfg *Config, from int) {
	// v3 reorganised the HUD into grouped sections with a new set of row keys,
	// so an older selection no longer refers to anything that exists.
	if from < 3 {
		cfg.Hud.Metrics = defaultHudMetrics()
		cfg.Hud.W, cfg.Hud.H = 320, 560
	}

	// v5 added the HUD sparklines and made the interface follow the Windows
	// display scaling. An install still pinned at 100% was only there because
	// that used to be the default, so move it onto the automatic behaviour.
	if from < 5 {
		if cfg.UiScale == 1 {
			cfg.UiScale = 0
		}
		for _, key := range []string{"frameGraph", "fpsGraph"} {
			if !slices.Contains(cfg.Hud.Metrics, key) {
				cfg.Hud.Metrics = append(cfg.Hud.Metrics, key)
			}
		}
		if cfg.Hud.H < 620 {
			cfg.Hud.H = 620 // room for the graphs
		}
	}

	// v6 dropped the WinRing0 opt-in and the LibreHardwareMonitor URL: the
	// sensor helper no longer installs a kernel driver, so there is nothing
	// left to opt into. Both keys simply disappear on the next save.
	//
	// v7 added the dismissible PawnIO prompt; v8 made the driver mandatory,
	// so the pawnIoPromptDismissed key disappears on the next save — the app
	// now blocks on the install gate instead of asking politely.
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}
