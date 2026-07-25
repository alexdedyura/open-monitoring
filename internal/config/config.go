package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	Version          int     `json:"version"`
	SampleIntervalMs int     `json:"sampleIntervalMs"`
	MaxRecordMinutes int     `json:"maxRecordMinutes"`
	LHMUrl           string  `json:"lhmUrl"` // fallback only; no UI
	Theme            string  `json:"theme"`   // dark | light
	UiScale          float64 `json:"uiScale"` // 0 = follow the Windows display scaling
	// EnableCpuSensors turns on the LibreHardwareMonitor bridge, which loads
	// the WinRing0 kernel driver to read CPU temperature/power. Microsoft
	// Defender flags that driver as vulnerable (CVE-2020-14979) and quarantines
	// it, so this stays OFF unless the user explicitly opts in.
	EnableCpuSensors bool      `json:"enableCpuSensors"`
	Hud              HudConfig `json:"hud"`
}

const currentVersion = 5

// defaultHudMetrics mirrors a classic in-game OSD: GPU block, CPU block, RAM
// block and the FPS block, each rendered as a titled section in the HUD.
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
		LHMUrl:           "http://localhost:8085/data.json",
		Theme:            "dark",
		UiScale:          0, // match Windows
		EnableCpuSensors: false,
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
	case 0, 1, 1.25, 1.5, 2:
	default:
		cfg.UiScale = 0
	}
	switch cfg.Hud.Anchor {
	case "free", "tl", "tr", "bl", "br":
	default:
		cfg.Hud.Anchor = "free"
	}
	if cfg.Version < currentVersion {
		// The HUD switched to grouped sections with a new metric set (v2), then
		// gained the FPS section (v3); reset the row selection once for those.
		if cfg.Version < 3 {
			cfg.Hud.Metrics = defaultHudMetrics()
			cfg.Hud.W = 320
			cfg.Hud.H = 560
		}
		// v4 only introduces EnableCpuSensors, which correctly defaults to
		// false for existing installs — nothing else to migrate.
		//
		// v5 makes the interface follow the Windows display scaling. Installs
		// that never changed the scale were pinned at 100% purely because that
		// was the old default, so move them onto the new automatic behaviour.
		if cfg.Version < 5 {
			if cfg.UiScale == 1 {
				cfg.UiScale = 0
			}
			// v5 also adds the HUD sparklines; switch them on for everyone.
			for _, k := range []string{"frameGraph", "fpsGraph"} {
				if !slices.Contains(cfg.Hud.Metrics, k) {
					cfg.Hud.Metrics = append(cfg.Hud.Metrics, k)
				}
			}
			if cfg.Hud.H < 620 {
				cfg.Hud.H = 620 // room for the graphs
			}
		}
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
