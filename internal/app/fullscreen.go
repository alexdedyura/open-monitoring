package app

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/metrics"
)

// A game in true exclusive fullscreen is presented past the desktop, so the
// HUD — or any other window — cannot appear over it, and from the player's
// side that looks exactly like a broken overlay. PresentMon reports the mode
// per frame, so the app says so instead: the HUD and the dashboard explain it
// (the frontend reads FPSMetrics.Exclusive), and a toast is raised once per
// game, because while the game holds the screen it is the one surface that
// will still be waiting for the player afterwards.

// noteExclusive raises that toast the first time a game is seen in exclusive
// fullscreen during this run.
func (a *App) noteExclusive(m *metrics.FPSMetrics) {
	if m == nil || !m.Exclusive || m.Process == "" {
		return
	}
	if a.exclusiveToasted == nil {
		a.exclusiveToasted = map[string]bool{}
	}
	if a.exclusiveToasted[m.Process] {
		return
	}
	a.exclusiveToasted[m.Process] = true
	go pushToast("Open Monitoring", exclusiveText(a.toastLang(), m.Process, m.FSOOff != ""))
}

// exclusiveText words that toast. Like alertText, it is translated here
// because Windows draws it and the frontend never sees it.
func exclusiveText(lang, process string, fsoOff bool) string {
	if lang == "ru" {
		s := fmt.Sprintf("%s работает в эксклюзивном полноэкранном режиме — поверх него HUD не виден. "+
			"Выберите в настройках графики игры «Полноэкранный оконный» (без рамки).", process)
		if fsoOff {
			s += " Для этой игры отключены оптимизации во весь экран — их можно включить в Open Monitoring."
		}
		return s
	}
	s := fmt.Sprintf("%s runs in exclusive fullscreen, where the HUD cannot be shown. "+
		"Choose borderless (windowed) fullscreen in the game's video settings.", process)
	if fsoOff {
		s += " Fullscreen optimizations are off for this game — Open Monitoring can turn them back on."
	}
	return s
}

// EnableFPSCapture asks for administrator rights for frame capture again,
// after the UAC prompt was declined or the elevated helper stopped.
func (a *App) EnableFPSCapture() {
	if a.collector != nil {
		a.collector.ElevateFPS()
	}
}

// RestoreFullscreenOptimizations turns Windows' fullscreen optimisations back
// on — for one game ("app", with its exe path) or for every game ("global") —
// which is what lets an "exclusive fullscreen" game run in the flip mode the
// HUD can be drawn over. It returns "" on success, or an i18n key. The game
// must be restarted to pick the change up.
func (a *App) RestoreFullscreenOptimizations(kind, exe string) string {
	if err := metrics.RestoreFSO(kind, exe); err != nil {
		runtime.LogErrorf(a.ctx, "restore fullscreen optimizations (%s, %q): %v", kind, exe, err)
		return "fso.restoreFailed"
	}
	return ""
}
