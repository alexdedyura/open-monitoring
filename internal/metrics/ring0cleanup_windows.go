//go:build windows

package metrics

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// LibreHardwareMonitor installs WinRing0 as a kernel service named "R0" plus
// the host process name, and drops the driver next to the executable. When the
// CPU-sensor feature is switched off we remove both, so a driver Defender flags
// as vulnerable is not left registered and auto-starting on the machine.
const ring0ServiceName = "R0lhm-bridge"

// CleanupRing0Driver stops and deletes our WinRing0 service and its .sys file.
// Everything is best-effort: without elevation, or with the file already
// quarantined by an antivirus, the calls simply fail and are ignored.
func CleanupRing0Driver() {
	removeRing0Service()

	if path := FindBridge(); path != "" {
		sys := filepath.Join(filepath.Dir(path), "lhm-bridge.sys")
		os.Remove(sys)
	}
	if exe, err := os.Executable(); err == nil {
		os.Remove(filepath.Join(filepath.Dir(exe), "lhm-bridge.sys"))
	}
}

func removeRing0Service() {
	m, err := mgr.Connect()
	if err != nil {
		return // not elevated
	}
	defer m.Disconnect()

	s, err := m.OpenService(ring0ServiceName)
	if err != nil {
		return // nothing registered
	}
	defer s.Close()

	if status, err := s.Control(svc.Stop); err == nil {
		deadline := time.Now().Add(3 * time.Second)
		for status.State != svc.Stopped && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			if status, err = s.Query(); err != nil {
				break
			}
		}
	}
	s.Delete()
}
