// Open Monitoring — a PC monitoring app for Windows: live dashboard,
// recordable sessions, and an always-on-top HUD overlay for games.
//
// This file is only the Wails wiring; the application lives in internal/app.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"open-monitoring/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := app.New()

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
		},
	})
	if err != nil {
		log.Fatalf("open-monitoring: %v", err)
	}
}
