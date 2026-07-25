package app

import (
	"encoding/base64"
	"errors"
	"os"
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

// ExportSessionPNG saves an already-rendered snapshot of a session's charts.
// The frontend renders the image — the charts only exist there — and passes
// it as base64 PNG; this side asks where to save and writes the bytes. The
// returned path is empty when the user cancelled the dialog.
func (a *App) ExportSessionPNG(id int64, pngBase64 string) (string, error) {
	if a.store == nil {
		return "", errNoStore
	}

	name, err := a.store.SessionName(id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: sanitizeFilename(name) + ".png",
		Filters:         []runtime.FileFilter{{DisplayName: "PNG image", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

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
