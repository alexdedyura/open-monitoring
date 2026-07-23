package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type HudConfig struct {
	Metrics          []string `json:"metrics"` // enabled metric keys, grouped into sections by the UI
	Opacity          float64  `json:"opacity"` // 0.2 .. 1
	Anchor           string   `json:"anchor"`  // free | tl | tr | bl | br
	X                int      `json:"x"`
	Y                int      `json:"y"`
	W                int      `json:"w"`
	H                int      `json:"h"`
	FsAlertDismissed bool     `json:"fsAlertDismissed"` // "exclusive fullscreen hides the HUD" notice
}

type Config struct {
	Version          int       `json:"version"`
	SampleIntervalMs int       `json:"sampleIntervalMs"`
	MaxRecordMinutes int       `json:"maxRecordMinutes"`
	LHMUrl           string    `json:"lhmUrl"` // fallback only; no UI
	Theme            string    `json:"theme"`  // dark | light
	UiScale          float64   `json:"uiScale"`
	Hud              HudConfig `json:"hud"`
}

const currentVersion = 3

// defaultHudMetrics mirrors a classic in-game OSD: GPU block, CPU block, RAM
// block and the FPS block, each rendered as a titled section in the HUD.
func defaultHudMetrics() []string {
	return []string{
		"gpu", "gpuTemp", "gpuClock", "gpuMemClock", "gpuPow", "vram",
		"cpu", "cpuTemp", "cpuClock", "cpuPow",
		"ram", "ramLoad", "ramSpeed",
		"fps", "fpsAvg", "fpsLow1", "fpsLow01",
	}
}

func Default() Config {
	return Config{
		Version:          currentVersion,
		SampleIntervalMs: 1000,
		MaxRecordMinutes: 240,
		LHMUrl:           "http://localhost:8085/data.json",
		Theme:            "dark",
		UiScale:          1,
		Hud: HudConfig{
			Metrics: defaultHudMetrics(),
			Opacity: 0.85,
			Anchor:  "free",
			X:       40,
			Y:       40,
			W:       320,
			H:       560,
		},
	}
}

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

func Load() Config {
	cfg := Default()
	// A file that predates the version field must read as v0 so migrations
	// run; the JSON value overrides this for current files.
	cfg.Version = 0
	data, err := os.ReadFile(path())
	if err != nil {
		cfg.Version = currentVersion
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default()
	}
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
	case 1, 1.25, 1.5, 2:
	default:
		cfg.UiScale = 1
	}
	switch cfg.Hud.Anchor {
	case "free", "tl", "tr", "bl", "br":
	default:
		cfg.Hud.Anchor = "free"
	}
	// The HUD switched to grouped sections with a new metric set (v2), then
	// gained the FPS section (v3); reset the row selection once per upgrade.
	if cfg.Version < currentVersion {
		cfg.Hud.Metrics = defaultHudMetrics()
		cfg.Hud.W = 320
		cfg.Hud.H = 560
		cfg.Version = currentVersion
		Save(cfg)
	}
	return cfg
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}
