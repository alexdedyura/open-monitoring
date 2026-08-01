package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/metrics"
)

// Threshold alerts. Every sample is checked against the configured limits; a
// rule fires once when its value crosses the threshold and cannot fire again
// until the value has dropped a little below it (hysteresis), with a cooldown
// on top so a value sitting right on the line does not machine-gun toasts.
//
// Evaluation lives on the backend rather than in the dashboard so alerts work
// while the window shows another tab, the HUD, or nothing at all.

const (
	alertCooldown   = 10 * time.Minute
	alertHysteresis = 3 // units (°C or %) below the threshold to re-arm
)

// AlertEvent is one firing, shown as an in-app toast and a Windows one.
//
// The wording travels as a catalogue key plus its numbers, not as a sentence:
// the in-app toast is rendered by the frontend, which is the side that knows
// which language the user picked. Message stays as the English rendering —
// it is the fallback if a key is ever missing, and it is what the Windows
// toast needs, because that one is drawn by the OS and never reaches the UI.
type AlertEvent struct {
	ID        string  `json:"id"`
	Key       string  `json:"key"`    // catalogue key, e.g. "alert.cpuTemp"
	Name      string  `json:"name"`   // the volume for disk rules, else empty
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	At        int64   `json:"at"` // unix milliseconds
}

type alertState struct {
	firing   bool
	notified time.Time
}

// checkAlerts runs on the collector's goroutine for every sample.
func (a *App) checkAlerts(s metrics.Sample) {
	cfg := a.cfg.Alerts
	if !cfg.Enabled {
		return
	}

	a.eval("cpuTemp", "alert.cpuTemp", "", s.CPU.TempC, cfg.CPUTempC)
	if s.GPU != nil {
		a.eval("gpuTemp", "alert.gpuTemp", "", s.GPU.TempC, cfg.GPUTempC)
	}
	a.eval("ram", "alert.ram", "", s.Mem.UsedPercent, cfg.RAMPercent)

	// Volumes alert independently: a full C: is not cured by an empty D:.
	for _, d := range s.Disks {
		a.eval("disk:"+d.Name, "alert.disk", d.Name, d.UsedPercent, cfg.DiskPercent)
	}
}

// alertText renders an alert for the Windows toast. Only this one surface needs
// a translation on the Go side — everything else in the app is worded by the
// frontend — so it is a table rather than machinery, and it carries only the
// four sentences that exist.
func alertText(lang, key, name string, v, threshold float64) string {
	if lang == "ru" {
		switch key {
		case "alert.cpuTemp":
			return fmt.Sprintf("Температура CPU %.0f °C — предел %.0f °C", v, threshold)
		case "alert.gpuTemp":
			return fmt.Sprintf("Температура GPU %.0f °C — предел %.0f °C", v, threshold)
		case "alert.ram":
			return fmt.Sprintf("Загрузка RAM %.0f%% — предел %.0f%%", v, threshold)
		case "alert.disk":
			return fmt.Sprintf("Накопитель %s заполнен на %.0f%% — предел %.0f%%", name, v, threshold)
		}
	}
	switch key {
	case "alert.cpuTemp":
		return fmt.Sprintf("CPU temperature %.0f °C — limit is %.0f °C", v, threshold)
	case "alert.gpuTemp":
		return fmt.Sprintf("GPU temperature %.0f °C — limit is %.0f °C", v, threshold)
	case "alert.ram":
		return fmt.Sprintf("RAM usage %.0f%% — limit is %.0f%%", v, threshold)
	case "alert.disk":
		return fmt.Sprintf("Drive %s is %.0f%% full — limit is %.0f%%", name, v, threshold)
	}
	return key
}

// toastLang resolves the configured language the same way the frontend does, so
// a system notification cannot arrive in a different language from the window
// that raised it.
func (a *App) toastLang() string {
	if a.cfg.Lang == "en" || a.cfg.Lang == "ru" {
		return a.cfg.Lang
	}
	if strings.HasPrefix(strings.ToLower(a.staticInfo.OSLang), "ru") {
		return "ru"
	}
	return "en"
}

// eval applies the fire/re-arm state machine for one rule. A zero threshold
// means the rule is off; a zero value means the sensor has no reading.
func (a *App) eval(id, key, name string, v, threshold float64) {
	if threshold <= 0 || v <= 0 {
		return
	}

	a.alertMu.Lock()
	st := a.alerts[id]
	if st == nil {
		st = &alertState{}
		a.alerts[id] = st
	}

	fire := false
	switch {
	case v >= threshold && (!st.firing || time.Since(st.notified) >= alertCooldown):
		st.firing = true
		st.notified = time.Now()
		fire = true
	case v < threshold-alertHysteresis:
		st.firing = false
	}
	a.alertMu.Unlock()

	if fire {
		event := AlertEvent{
			ID:        id,
			Key:       key,
			Name:      name,
			Message:   alertText("en", key, name, v, threshold),
			Value:     v,
			Threshold: threshold,
			At:        time.Now().UnixMilli(),
		}
		runtime.EventsEmit(a.ctx, "alert", event)
		// The system toast reaches the user when the app is behind a game or
		// minimised — precisely when an alert matters most. It is drawn by
		// Windows, so this is the one place the wording is settled here.
		go pushToast("Open Monitoring", alertText(a.toastLang(), key, name, v, threshold))
	}
}
