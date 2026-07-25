package app

import (
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/metrics"
)

// errNoStore is returned when the session database could not be opened at
// startup. Everything else in the app still works, so this is reported to the
// user rather than being fatal.
var errNoStore = errors.New("session storage unavailable")

// flushEvery is how many samples are buffered before being written. Batching
// keeps a long recording from doing one SQLite transaction per second.
const flushEvery = 10

// recording is the state of the session currently being captured.
type recording struct {
	sessionID int64
	name      string
	startedAt int64 // unix milliseconds
	count     int
	buf       []metrics.Sample
}

// RecStatus is what the frontend sees about the current recording.
type RecStatus struct {
	Active     bool   `json:"active"`
	SessionID  int64  `json:"sessionId"`
	Name       string `json:"name"`
	StartedAt  int64  `json:"startedAt"`
	Count      int    `json:"count"`
	MaxMinutes int    `json:"maxMinutes"`
}

// StartRecording begins capturing samples into a new session. Calling it while
// a recording is already running is a no-op that returns the running one.
func (a *App) StartRecording(name string) (RecStatus, error) {
	a.recMu.Lock()
	defer a.recMu.Unlock()

	if a.rec != nil {
		return a.statusLocked(), nil
	}
	if a.store == nil {
		return RecStatus{}, errNoStore
	}
	if name == "" {
		name = "Session " + time.Now().Format("2006-01-02 15:04")
	}

	startedAt := time.Now().UnixMilli()
	id, err := a.store.CreateSession(name, startedAt, a.cfg.SampleIntervalMs)
	if err != nil {
		return RecStatus{}, err
	}
	a.rec = &recording{sessionID: id, name: name, startedAt: startedAt}

	status := a.statusLocked()
	runtime.EventsEmit(a.ctx, "recording", status)
	return status, nil
}

// StopRecording ends the current recording, flushing anything still buffered.
// Safe to call when nothing is being recorded.
func (a *App) StopRecording() RecStatus {
	a.recMu.Lock()
	defer a.recMu.Unlock()

	a.stopLocked()
	status := a.statusLocked()
	if a.ctx != nil { // nil during shutdown before startup completed
		runtime.EventsEmit(a.ctx, "recording", status)
	}
	return status
}

func (a *App) GetRecStatus() RecStatus {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	return a.statusLocked()
}

// record appends one sample to the running recording, flushing in batches and
// stopping automatically once the configured time cap is reached.
func (a *App) record(s metrics.Sample) {
	a.recMu.Lock()
	defer a.recMu.Unlock()

	if a.rec == nil {
		return
	}

	a.rec.buf = append(a.rec.buf, s)
	a.rec.count++
	if len(a.rec.buf) >= flushEvery {
		a.flushLocked()
	}

	capMs := int64(a.cfg.MaxRecordMinutes) * 60_000
	if s.T-a.rec.startedAt >= capMs {
		a.stopLocked()
		runtime.EventsEmit(a.ctx, "recording", a.statusLocked())
	}
}

func (a *App) flushLocked() {
	if a.store == nil || a.rec == nil || len(a.rec.buf) == 0 {
		return
	}
	if err := a.store.AppendSamples(a.rec.sessionID, a.rec.buf); err != nil {
		runtime.LogErrorf(a.ctx, "append samples: %v", err)
	}
	a.rec.buf = a.rec.buf[:0]
}

func (a *App) stopLocked() {
	if a.rec == nil {
		return
	}
	a.flushLocked()
	if a.store != nil {
		a.store.EndSession(a.rec.sessionID, time.Now().UnixMilli())
	}
	a.rec = nil
}

func (a *App) statusLocked() RecStatus {
	status := RecStatus{MaxMinutes: a.cfg.MaxRecordMinutes}
	if a.rec != nil {
		status.Active = true
		status.SessionID = a.rec.sessionID
		status.Name = a.rec.name
		status.StartedAt = a.rec.startedAt
		status.Count = a.rec.count
	}
	return status
}

// csvColumns is the header of an exported session, and defines the order the
// rows below are written in.
var csvColumns = []string{
	"time", "cpu_usage_pct", "cpu_temp_c", "cpu_power_w", "cpu_clock_mhz",
	"ram_used_mb", "ram_used_pct", "swap_used_mb", "swap_total_mb",
	"gpu_usage_pct", "gpu_temp_c", "gpu_hotspot_c", "vram_used_mb",
	"gpu_power_w", "gpu_core_mhz", "gpu_fan_pct",
	"disk_read_kbps", "disk_write_kbps", "net_up_kbps", "net_down_kbps",
	"fps_cur", "fps_avg", "fps_low1", "fps_low01",
}

// ExportSessionCSV asks the user for a target path and writes the session to
// it. The returned path is empty when the user cancelled the dialog.
func (a *App) ExportSessionCSV(id int64) (string, error) {
	if a.store == nil {
		return "", errNoStore
	}

	name, err := a.store.SessionName(id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: sanitizeFilename(name) + ".csv",
		Filters:         []runtime.FileFilter{{DisplayName: "CSV", Pattern: "*.csv"}},
	})
	if err != nil || path == "" {
		return "", err
	}

	samples, err := a.store.SessionSamples(id)
	if err != nil {
		return "", err
	}

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(csvColumns); err != nil {
		return "", err
	}
	for _, s := range samples {
		if err := w.Write(csvRow(s)); err != nil {
			return "", err
		}
	}
	return path, w.Error()
}

func csvRow(s metrics.Sample) []string {
	var readBps, writeBps float64
	for _, d := range s.Disks {
		readBps += d.ReadBps
		writeBps += d.WriteBps
	}

	// Optional sections are absent for samples recorded without a GPU or
	// without PresentMon; a zero value exports as 0, which reads correctly as
	// "nothing measured" in a spreadsheet.
	gpu := s.GPU
	if gpu == nil {
		gpu = &metrics.GPUMetrics{}
	}
	fps := s.FPS
	if fps == nil {
		fps = &metrics.FPSMetrics{}
	}

	const kb, mb = 1024, 1024 * 1024
	return []string{
		time.UnixMilli(s.T).Format("2006-01-02 15:04:05"),
		num(s.CPU.Usage), num(s.CPU.TempC), num(s.CPU.PowerW), num(s.CPU.ClockMHz),
		num(float64(s.Mem.Used) / mb), num(s.Mem.UsedPercent),
		num(float64(s.Mem.SwapUsed) / mb), num(float64(s.Mem.SwapTotal) / mb),
		num(gpu.Usage), num(gpu.TempC), num(gpu.HotspotC), num(gpu.MemUsedMB),
		num(gpu.PowerW), num(gpu.CoreMHz), num(gpu.FanPercent),
		num(readBps / kb), num(writeBps / kb), num(s.Net.UpBps / kb), num(s.Net.DownBps / kb),
		num(fps.Cur), num(fps.Avg), num(fps.Low1), num(fps.Low01),
	}
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// sanitizeFilename replaces the characters Windows forbids in a file name, so
// a session named "GTA V: 12/07" still produces a valid default filename.
func sanitizeFilename(s string) string {
	out := []rune(s)
	for i, r := range out {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			out[i] = '_'
		}
	}
	return string(out)
}
