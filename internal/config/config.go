package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type HudConfig struct {
	Metrics []string `json:"metrics"` // ordered metric keys shown in the HUD
	Opacity float64  `json:"opacity"` // 0.2 .. 1
	X       int      `json:"x"`
	Y       int      `json:"y"`
	W       int      `json:"w"`
	H       int      `json:"h"`
}

type Config struct {
	SampleIntervalMs int       `json:"sampleIntervalMs"`
	MaxRecordMinutes int       `json:"maxRecordMinutes"`
	LHMUrl           string    `json:"lhmUrl"`
	Theme            string    `json:"theme"` // dark | light
	Hud              HudConfig `json:"hud"`
}

func Default() Config {
	return Config{
		SampleIntervalMs: 1000,
		MaxRecordMinutes: 240,
		LHMUrl:           "http://localhost:8085/data.json",
		Theme:            "dark",
		Hud: HudConfig{
			Metrics: []string{"cpu", "cpuTemp", "ram", "gpu", "gpuTemp", "vram", "net"},
			Opacity: 0.85,
			X:       40,
			Y:       40,
			W:       300,
			H:       320,
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
	data, err := os.ReadFile(path())
	if err != nil {
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
	return cfg
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}
