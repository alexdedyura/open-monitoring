package metrics

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BridgeSource runs the bundled lhm-bridge.exe (a ~80-line C# worker over
// LibreHardwareMonitorLib) and consumes its stdout JSON stream. This embeds
// LibreHardwareMonitor into the app: no external install needed. Sensor
// coverage grows when the app runs elevated (CPU temperature needs the
// WinRing0 driver, which requires admin).
type BridgeSource struct {
	mu     sync.Mutex
	latest *LHMReading
	cmd    *exec.Cmd
	closed bool
}

type bridgeDump struct {
	HW []struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Sensors []struct {
			T string  `json:"t"`
			N string  `json:"n"`
			V float64 `json:"v"`
		} `json:"sensors"`
	} `json:"hw"`
}

// FindBridge looks for lhm-bridge.exe next to the running executable first
// (packaged layout), then in the repo tree (development).
func FindBridge() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "lhm-bridge.exe"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "tools", "lhm-bridge", "publish", "lhm-bridge.exe"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func StartBridge(path string) *BridgeSource {
	b := &BridgeSource{}
	go b.loop(path)
	return b
}

func (b *BridgeSource) Latest() *LHMReading {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.latest
}

func (b *BridgeSource) Stop() {
	b.mu.Lock()
	b.closed = true
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	b.mu.Unlock()
}

func (b *BridgeSource) loop(path string) {
	// Restarting on a fixed short interval turns a persistently failing bridge
	// into a storm: every start re-installs the WinRing0 driver, which an
	// antivirus may quarantine, and the retry loop then hammers the machine.
	// Back off, and give up entirely once the bridge clearly cannot run.
	backoff := 10 * time.Second
	const maxBackoff = 5 * time.Minute
	failures := 0
	const maxFailures = 5

	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()

		startedAt := time.Now()
		cmd := exec.Command(path, "2000", strconv.Itoa(os.Getpid()))
		hideWindow(cmd)
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			failures++
			if failures >= maxFailures {
				return
			}
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}
		b.mu.Lock()
		b.cmd = cmd
		b.mu.Unlock()

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 256*1024), 1024*1024)
		for sc.Scan() {
			if r := parseBridgeLine(sc.Bytes()); r != nil {
				b.mu.Lock()
				b.latest = r
				b.mu.Unlock()
			}
		}
		cmd.Wait()
		b.mu.Lock()
		b.latest = nil
		closed := b.closed
		b.mu.Unlock()
		if closed {
			return
		}

		// A bridge that died almost immediately never produced sensors — most
		// likely its driver was blocked. Count those; a long-lived run that
		// merely crashed resets the budget.
		if time.Since(startedAt) < 20*time.Second {
			failures++
			if failures >= maxFailures {
				return
			}
		} else {
			failures = 0
			backoff = 10 * time.Second
		}
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

func parseBridgeLine(line []byte) *LHMReading {
	var d bridgeDump
	if err := json.Unmarshal(line, &d); err != nil {
		return nil
	}
	r := &LHMReading{}
	for _, hw := range d.HW {
		get := func(sensorType string, prefer ...string) float64 {
			for _, p := range prefer {
				for _, s := range hw.Sensors {
					if s.T == sensorType && strings.Contains(s.N, p) {
						return s.V
					}
				}
			}
			return 0
		}
		maxOf := func(sensorType, contains string) float64 {
			m := 0.0
			for _, s := range hw.Sensors {
				if s.T == sensorType && strings.Contains(s.N, contains) && s.V > m {
					m = s.V
				}
			}
			return m
		}

		switch {
		case hw.Type == "Cpu":
			if r.CPUTemp == 0 {
				r.CPUTemp = get("Temperature", "CPU Package", "Core (Tctl/Tdie)", "Core Max", "Average")
				if r.CPUTemp == 0 {
					r.CPUTemp = maxOf("Temperature", "Core")
				}
			}
			if r.CPUPower == 0 {
				r.CPUPower = get("Power", "CPU Package", "Package")
			}
			if r.CPUClock == 0 {
				r.CPUClock = maxOf("Clock", "Core")
			}
		case strings.HasPrefix(hw.Type, "Gpu"):
			if r.GPU == nil {
				g := &GPUMetrics{
					Usage:      get("Load", "GPU Core"),
					TempC:      get("Temperature", "GPU Core"),
					PowerW:     get("Power", "GPU Package", "GPU Power"),
					CoreMHz:    get("Clock", "GPU Core"),
					MemMHz:     get("Clock", "GPU Memory"),
					MemUsedMB:  get("SmallData", "GPU Memory Used"),
					MemTotalMB: get("SmallData", "GPU Memory Total"),
					FanPercent: get("Control", "GPU Fan"),
				}
				if g.Usage > 0 || g.TempC > 0 {
					r.GPUName = hw.Name
					r.GPU = g
				}
			}
		case hw.Type == "Storage":
			sh := StorageHealth{
				Name:          hw.Name,
				TempC:         get("Temperature", "Temperature"),
				DataWrittenGB: get("Data", "Data Written"),
			}
			if v := get("Level", "Remaining Life"); v > 0 {
				sh.LifePercent = v
			} else if v := get("Level", "Percentage Used"); v > 0 {
				sh.LifePercent = 100 - v
			}
			r.Storage = append(r.Storage, sh)
		}
	}
	if r.CPUTemp == 0 && r.CPUPower == 0 && r.GPU == nil && len(r.Storage) == 0 {
		return nil
	}
	return r
}
