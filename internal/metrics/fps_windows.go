//go:build windows

package metrics

import (
	"bufio"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FPSSource streams frame times from Intel PresentMon (--output_stdout) and
// aggregates them per process. FPS is reported for the foreground window's
// process. Needs elevation (ETW trace session) — without it the spawn fails
// and FPS metrics simply stay absent.
type FPSSource struct {
	mu     sync.Mutex
	rings  map[uint32]*frameRing
	names  map[uint32]string
	cmd    *exec.Cmd
	closed bool
}

type frameEntry struct {
	at time.Time
	ms float64
}

type frameRing struct {
	entries []frameEntry
}

const fpsWindow = 60 * time.Second

// FindPresentMon looks next to the app executable, then in the repo tree.
func FindPresentMon() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "PresentMon.exe"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "tools", "presentmon", "PresentMon.exe"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func StartFPS() *FPSSource {
	path := FindPresentMon()
	if path == "" {
		return nil
	}
	f := &FPSSource{rings: map[uint32]*frameRing{}, names: map[uint32]string{}}
	go f.loop(path)
	return f
}

func (f *FPSSource) Stop() {
	f.mu.Lock()
	f.closed = true
	if f.cmd != nil && f.cmd.Process != nil {
		f.cmd.Process.Kill()
	}
	f.mu.Unlock()
}

func (f *FPSSource) loop(path string) {
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return
		}
		f.mu.Unlock()

		cmd := exec.Command(path,
			"--output_stdout",
			"--stop_existing_session",
			"--session_name", "OpenMonitoring")
		hideWindow(cmd)
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			time.Sleep(60 * time.Second) // likely not elevated; retry rarely
			continue
		}
		f.mu.Lock()
		f.cmd = cmd
		f.mu.Unlock()

		f.consume(stdout)
		cmd.Wait()

		f.mu.Lock()
		closed := f.closed
		f.rings = map[uint32]*frameRing{}
		f.mu.Unlock()
		if closed {
			return
		}
		time.Sleep(30 * time.Second)
	}
}

func (f *FPSSource) consume(r interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	iApp, iPid, iMs := -1, -1, -1
	for sc.Scan() {
		line := sc.Text()
		if iMs < 0 || strings.HasPrefix(line, "Application") {
			// header (PresentMon may re-emit it); column names differ by version
			cols := strings.Split(line, ",")
			for i, c := range cols {
				switch strings.ToLower(strings.TrimSpace(c)) {
				case "application":
					iApp = i
				case "processid":
					iPid = i
				case "msbetweenpresents", "frametime", "msbetweendisplaychange":
					if iMs < 0 || strings.EqualFold(c, "MsBetweenPresents") || strings.EqualFold(c, "FrameTime") {
						iMs = i
					}
				}
			}
			continue
		}
		cols := strings.Split(line, ",")
		if iApp >= len(cols) || iPid >= len(cols) || iMs >= len(cols) {
			continue
		}
		pid64, err := strconv.ParseUint(strings.TrimSpace(cols[iPid]), 10, 32)
		if err != nil {
			continue
		}
		ms, err := strconv.ParseFloat(strings.TrimSpace(cols[iMs]), 64)
		if err != nil || ms <= 0 || math.IsNaN(ms) {
			continue
		}
		pid := uint32(pid64)
		now := time.Now()

		f.mu.Lock()
		ring := f.rings[pid]
		if ring == nil {
			ring = &frameRing{}
			f.rings[pid] = ring
			f.names[pid] = strings.TrimSuffix(strings.TrimSpace(cols[iApp]), ".exe")
		}
		ring.entries = append(ring.entries, frameEntry{at: now, ms: ms})
		f.mu.Unlock()
	}
}

// Metrics computes FPS stats for the foreground process (or, failing that,
// the process presenting the most frames).
func (f *FPSSource) Metrics() *FPSMetrics {
	now := time.Now()
	fg := foregroundPID()
	self := uint32(os.Getpid())

	f.mu.Lock()
	defer f.mu.Unlock()

	// prune old frames and drop idle processes
	for pid, ring := range f.rings {
		cut := 0
		for cut < len(ring.entries) && now.Sub(ring.entries[cut].at) > fpsWindow {
			cut++
		}
		ring.entries = ring.entries[cut:]
		if len(ring.entries) == 0 {
			delete(f.rings, pid)
			delete(f.names, pid)
		}
	}

	pick := uint32(0)
	if fg != 0 && fg != self {
		if ring := f.rings[fg]; ring != nil && len(ring.entries) >= 8 {
			pick = fg
		}
	}
	if pick == 0 {
		best := 0
		for pid, ring := range f.rings {
			name := strings.ToLower(f.names[pid])
			if pid == self || name == "dwm" || name == "explorer" {
				continue
			}
			if n := len(ring.entries); n > best && n >= 30 {
				best = n
				pick = pid
			}
		}
	}
	if pick == 0 {
		return nil
	}

	entries := f.rings[pick].entries
	msAll := make([]float64, 0, len(entries))
	var sumAll, sumCur float64
	nCur := 0
	for _, e := range entries {
		msAll = append(msAll, e.ms)
		sumAll += e.ms
		if now.Sub(e.at) <= time.Second {
			sumCur += e.ms
			nCur++
		}
	}

	m := &FPSMetrics{Process: f.names[pick]}
	if nCur > 0 {
		m.Cur = round1(1000 / (sumCur / float64(nCur)))
	}
	m.Avg = round1(1000 / (sumAll / float64(len(msAll))))

	sort.Float64s(msAll) // ascending; worst frames at the end
	if worst := meanTail(msAll, 0.01); worst > 0 {
		m.Low1 = round1(1000 / worst)
	}
	if worst := meanTail(msAll, 0.001); worst > 0 {
		m.Low01 = round1(1000 / worst)
	}
	return m
}

// meanTail averages the worst `frac` share of sorted-ascending frame times.
func meanTail(sortedMs []float64, frac float64) float64 {
	n := int(math.Ceil(float64(len(sortedMs)) * frac))
	if n < 1 {
		n = 1
	}
	if n > len(sortedMs) {
		return 0
	}
	sum := 0.0
	for _, v := range sortedMs[len(sortedMs)-n:] {
		sum += v
	}
	return sum / float64(n)
}

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

func foregroundPID() uint32 {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}
