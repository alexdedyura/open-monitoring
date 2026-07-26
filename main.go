// Open Monitoring — a PC monitoring app for Windows: live dashboard,
// recordable sessions, and an always-on-top HUD overlay for games.
//
// This file is only the Wails wiring; the application lives in internal/app.
package main

import (
	"embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"open-monitoring/internal/app"
	"open-monitoring/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

// wails.json is the single place the version lives: it stamps the exe's file
// properties, the installer pages, the About tab — and, embedded here, the
// update check.
//
//go:embed wails.json
var wailsJSON []byte

func appVersion() string {
	var cfg struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	json.Unmarshal(wailsJSON, &cfg)
	return cfg.Info.ProductVersion
}

func main() {
	// A copy relaunched by the updater starts while the old instance is still
	// shutting down; waiting lets it release what only one instance can hold —
	// the global hotkeys and the session database.
	if os.Getenv("OPEN_MONITORING_RELAUNCH") == "1" {
		time.Sleep(2 * time.Second)
	}

	a := app.New(appVersion())

	err := wails.Run(&options.App{
		Title:     "Open Monitoring",
		Width:     1280,
		Height:    840,
		MinWidth:  220, // the HUD overlay shrinks the same window
		MinHeight: 140,
		Frameless: true, // custom titlebar on the dashboard, no chrome on the HUD

		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},

		OnStartup:  a.Startup,
		OnShutdown: a.Shutdown,
		Bind:       []any{a},

		Windows: &windows.Options{
			WebviewIsTransparent: true, // the HUD overlay needs a see-through window
			WindowIsTranslucent:  true,
			BackdropType:         windows.None,
			DisableWindowIcon:    true,
			// Everything the app writes lives in the hidden folder next to the
			// exe — including the WebView2 cache, which would otherwise land in
			// %AppData%.
			WebviewUserDataPath: filepath.Join(config.Dir(), "webview"),
		},
	})
	if err != nil {
		log.Fatalf("open-monitoring: %v", err)
	}
}
