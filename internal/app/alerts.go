package app

import (
	"fmt"
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
type AlertEvent struct {
	ID        string  `json:"id"`
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

	a.eval("cpuTemp", s.CPU.TempC, cfg.CPUTempC,
		fmt.Sprintf("CPU temperature %.0f °C — limit is %.0f °C", s.CPU.TempC, cfg.CPUTempC))
	if s.GPU != nil {
		a.eval("gpuTemp", s.GPU.TempC, cfg.GPUTempC,
			fmt.Sprintf("GPU temperature %.0f °C — limit is %.0f °C", s.GPU.TempC, cfg.GPUTempC))
	}
	a.eval("ram", s.Mem.UsedPercent, cfg.RAMPercent,
		fmt.Sprintf("RAM usage %.0f%% — limit is %.0f%%", s.Mem.UsedPercent, cfg.RAMPercent))

	// Volumes alert independently: a full C: is not cured by an empty D:.
	for _, d := range s.Disks {
		a.eval("disk:"+d.Name, d.UsedPercent, cfg.DiskPercent,
			fmt.Sprintf("Drive %s is %.0f%% full — limit is %.0f%%", d.Name, d.UsedPercent, cfg.DiskPercent))
	}
}

// eval applies the fire/re-arm state machine for one rule. A zero threshold
// means the rule is off; a zero value means the sensor has no reading.
func (a *App) eval(id string, v, threshold float64, msg string) {
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
		event := AlertEvent{ID: id, Message: msg, Value: v, Threshold: threshold, At: time.Now().UnixMilli()}
		runtime.EventsEmit(a.ctx, "alert", event)
		// The system toast reaches the user when the app is behind a game or
		// minimised — precisely when an alert matters most.
		go pushToast("Open Monitoring", msg)
	}
}
