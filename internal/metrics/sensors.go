package metrics

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"open-monitoring/internal/sidecar"
)

// SensorSource runs the bundled lhm-bridge helper and keeps its most recent
// reading. The helper wraps LibreHardwareMonitor, which reaches vendor APIs
// (NVAPI, AMD ADL, Intel IGCL) and the PawnIO driver that Go cannot — see
// tools/lhm-bridge. It streams one JSON object per interval on stdout; this
// type owns the process lifecycle and the decoding.
type SensorSource struct {
	mu     sync.Mutex
	latest *SensorReading
	cmd    *exec.Cmd
	closed bool

	// restart wakes the run loop out of its backoff sleep. Buffered so a
	// request is never lost and never blocks the caller.
	restart chan struct{}
}

// SensorReading is one decoded line from the helper. Fields left at zero are
// sensors the machine does not expose, or that need a privilege the app does
// not currently have.
type SensorReading struct {
	// PawnIO reports whether the driver behind CPU package temperature and
	// power is installed on this machine.
	PawnIO        bool   `json:"pawnIo"`
	PawnIOVersion string `json:"pawnIoVersion"`

	CPU struct {
		TempC    float64 `json:"tempC"`
		PowerW   float64 `json:"powerW"`
		ClockMHz float64 `json:"clockMhz"`
	} `json:"cpu"`

	GPU *struct {
		Name string `json:"name"`
		GPUMetrics
	} `json:"gpu"`

	Storage []StorageHealth `json:"storage"`

	// System is the SMBIOS machine description. It needs no privileges and no
	// driver, so the helper reads it instead of a second WMI round trip.
	System *struct {
		Board  string   `json:"board"`
		Memory *RAMInfo `json:"memory"`
	} `json:"system"`
}

// StorageHealth is the wear data the helper reads from a drive's SMART data.
type StorageHealth struct {
	Name          string  `json:"name"`
	TempC         float64 `json:"tempC"`
	LifePercent   float64 `json:"lifePercent"`
	DataWrittenGb float64 `json:"dataWrittenGb"`
}

// sensorIntervalMs is how often the helper samples. Hardware sensors change
// slowly and some are expensive to read, so this stays well above the app's
// own sampling interval; the collector reuses the latest reading in between.
const sensorIntervalMs = 2000

// StartSensors launches the helper. It returns a usable source even when the
// helper is missing — Latest simply keeps reporting nil, and every sensor that
// depends on it stays blank.
func StartSensors() *SensorSource {
	s := &SensorSource{restart: make(chan struct{}, 1)}
	go s.run()
	return s
}

// Restart relaunches the helper as soon as possible. LibreHardwareMonitor
// decides once, at type initialisation, whether the PawnIO driver is present,
// so a helper started before the driver was installed never sees it — only a
// fresh process does.
func (s *SensorSource) Restart() {
	s.mu.Lock()
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill() // ends the current stream; run() then loops
	}
	s.mu.Unlock()

	select {
	case s.restart <- struct{}{}: // skip the backoff sleep
	default:
	}
}

// Latest returns the most recent reading, or nil when the helper is not
// running. The returned value is read-only and shared between callers.
func (s *SensorSource) Latest() *SensorReading {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func (s *SensorSource) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

// run keeps the helper alive, backing off between restarts. A helper that
// keeps dying on startup is failing for a reason retrying will not fix, so the
// loop gives up rather than respawning a process forever.
func (s *SensorSource) run() {
	path, err := sidecar.Path(sidecar.Bridge)
	if err != nil {
		return // not bundled in this build, and that will not change at runtime
	}

	const (
		minBackoff  = 5 * time.Second
		maxBackoff  = 5 * time.Minute
		healthyRun  = 30 * time.Second // a run this long counts as working
		maxFailures = 5
	)

	backoff := minBackoff
	failures := 0

	for !s.stopped() {
		startedAt := time.Now()
		s.streamOnce(path)

		if s.stopped() {
			return
		}

		// A crash after a long healthy run is a hiccup worth retrying; only
		// runs that die on startup count against the budget. An explicit
		// restart is not a failure at all.
		if requested := s.restartRequested(); requested || time.Since(startedAt) >= healthyRun {
			failures, backoff = 0, minBackoff
			if requested {
				continue // relaunch immediately
			}
		} else if failures++; failures >= maxFailures {
			return
		}

		select {
		case <-s.restart:
		case <-time.After(backoff):
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (s *SensorSource) restartRequested() bool {
	select {
	case <-s.restart:
		return true
	default:
		return false
	}
}

func (s *SensorSource) stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// streamOnce runs the helper until it exits, decoding readings as they arrive.
func (s *SensorSource) streamOnce(path string) {
	cmd := exec.Command(path, strconv.Itoa(sensorIntervalMs), strconv.Itoa(os.Getpid()))
	hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	s.consume(stdout)
	cmd.Wait()

	s.mu.Lock()
	s.latest = nil // readings are stale the moment the helper is gone
	s.mu.Unlock()
}

func (s *SensorSource) consume(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)

	for sc.Scan() {
		var r SensorReading
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue // a diagnostic line rather than a reading
		}
		s.mu.Lock()
		s.latest = &r
		s.mu.Unlock()
	}
}
