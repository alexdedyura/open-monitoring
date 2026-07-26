//go:build windows

package metrics

import (
	"bufio"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"open-monitoring/internal/sidecar"
)

// FPSSource measures frame rate the way overlays do: by consuming the present
// events Windows already emits, rather than hooking the game. Intel PresentMon
// turns those ETW events into a CSV stream on stdout; this type keeps a window
// of recent frame times per process and reports on the foreground one.
//
// The ETW trace session needs administrator rights. Without them PresentMon
// fails to start and FPS metrics stay absent — everything else keeps working.
type FPSSource struct {
	mu      sync.Mutex
	frames  map[uint32][]frameTime // pid -> recent frames
	names   map[uint32]string      // pid -> process name
	cmd     *exec.Cmd
	running bool
	closed  bool
	lastErr string // why PresentMon is not delivering, for Settings
}

type frameTime struct {
	at time.Time
	ms float64
}

// fpsWindow is how much history the averages and lows are computed over. A
// minute is long enough for 0.1% lows to mean something at typical frame rates.
const fpsWindow = 60 * time.Second

// curWindow is what "current FPS" averages over. Short enough that the readout
// reacts to a drop within half a second, long enough that it does not flicker
// between neighbouring frames.
const curWindow = 500 * time.Millisecond

// frameGraphWindow is the slice of history behind the frame-time graph point.
// The HUD refreshes faster than this, so a single slow frame stays visible for
// two consecutive points instead of being lost between them.
const frameGraphWindow = 150 * time.Millisecond

// staleAfter is how long the newest frame may be before the process counts as
// no longer presenting. It has to clear the gap between two of PresentMon's
// buffered writes, which at a low frame rate is around a second.
const staleAfter = 3 * time.Second

// maxFrameMs discards presentation gaps. MsBetweenPresents measures the time
// since the previous present, so alt-tabbing, minimising or a loading screen
// produces one "frame" that is seconds long. Left in, that single value is the
// worst frame in the window and pins the 1% and 0.1% lows to nonsense for the
// next minute. A frame slower than this is not a rendered frame, it is a gap.
const maxFrameMs = 1000

// minFramesForeground is how many frames the foreground process must have
// produced before it is treated as "the game" rather than an idle window.
const minFramesForeground = 8

// A low is the mean of the worst `frac` share of frames, so it only means
// something once the window holds enough frames for that share to be more than
// a single sample. Below these counts the 1% and 0.1% lows would both collapse
// onto the one worst frame, and the UI is better off showing nothing.
const (
	minFramesLow1  = 100
	minFramesLow01 = 1000
)

// StartFPS launches PresentMon. It always returns a source; when the helper is
// missing or cannot start, Metrics simply keeps returning nil.
func StartFPS() *FPSSource {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{},
	}
	go f.run()
	return f
}

// Running reports whether PresentMon is currently streaming.
func (f *FPSSource) Running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

// LastError explains why frame times are absent, empty while streaming works.
func (f *FPSSource) LastError() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastErr
}

func (f *FPSSource) setErr(msg string) {
	f.mu.Lock()
	f.lastErr = msg
	f.mu.Unlock()
}

// Reset restarts the average and the lows. They are cumulative over a minute
// of play, so after a loading screen or a settings change they describe a run
// the user is no longer interested in.
//
// The last half second of frames survives, because the current frame rate and
// the frame-time graph are not what is being reset — dropping those too would
// blank the readout and punch a hole in the graph on every press.
func (f *FPSSource) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for pid, frames := range f.frames {
		var sum float64
		keep := 0
		for i := len(frames) - 1; i >= 0 && sum < float64(curWindow.Milliseconds()); i-- {
			sum += frames[i].ms
			keep++
		}
		f.frames[pid] = append([]frameTime(nil), frames[len(frames)-keep:]...)
	}
}

func (f *FPSSource) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if f.cmd != nil && f.cmd.Process != nil {
		f.cmd.Process.Kill()
	}
}

func (f *FPSSource) run() {
	path, err := sidecar.Path(sidecar.PresentMon)
	if err != nil {
		f.setErr("PresentMon is not bundled in this build")
		return
	}

	// The usual reason for failing to start is missing elevation, which will
	// not change while the app runs — so retry a few times, then stop.
	const maxFailures = 3
	failures := 0

	for !f.stopped() {
		if f.streamOnce(path) {
			failures = 0
		} else if failures++; failures >= maxFailures {
			f.setErr("PresentMon could not start — frame capture needs administrator rights")
			return
		}
		if f.stopped() {
			return
		}
		time.Sleep(30 * time.Second)
	}
}

func (f *FPSSource) stopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// streamOnce runs PresentMon until it exits, reporting whether it started.
func (f *FPSSource) streamOnce(path string) bool {
	cmd := exec.Command(path,
		"--output_stdout",
		"--stop_existing_session",
		"--session_name", "OpenMonitoring")
	hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}

	f.mu.Lock()
	f.cmd, f.running, f.lastErr = cmd, true, ""
	f.mu.Unlock()

	f.consume(stdout)
	cmd.Wait()

	f.mu.Lock()
	f.running = false
	f.frames = map[uint32][]frameTime{} // stale the moment the stream stops
	f.mu.Unlock()

	return true
}

// consume parses PresentMon's CSV stream. Column names and order differ
// between PresentMon versions, so the header is resolved by name rather than
// by position — and re-resolved whenever a new header appears mid-stream.
func (f *FPSSource) consume(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)

	appCol, pidCol, msCol := -1, -1, -1

	for sc.Scan() {
		line := sc.Text()

		if msCol < 0 || strings.HasPrefix(line, "Application") {
			appCol, pidCol, msCol = parseFPSHeader(line)
			continue
		}

		cols := strings.Split(line, ",")
		if appCol >= len(cols) || pidCol >= len(cols) || msCol >= len(cols) {
			continue
		}
		pid, err := strconv.ParseUint(strings.TrimSpace(cols[pidCol]), 10, 32)
		if err != nil {
			continue
		}
		ms, err := strconv.ParseFloat(strings.TrimSpace(cols[msCol]), 64)
		if err != nil || ms <= 0 || math.IsNaN(ms) || ms > maxFrameMs {
			continue
		}

		f.mu.Lock()
		if _, seen := f.frames[uint32(pid)]; !seen {
			f.names[uint32(pid)] = strings.TrimSuffix(strings.TrimSpace(cols[appCol]), ".exe")
		}
		f.frames[uint32(pid)] = append(f.frames[uint32(pid)], frameTime{at: time.Now(), ms: ms})
		f.mu.Unlock()
	}
}

func parseFPSHeader(line string) (appCol, pidCol, msCol int) {
	appCol, pidCol, msCol = -1, -1, -1
	for i, name := range strings.Split(line, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "application":
			appCol = i
		case "processid":
			pidCol = i
		case "msbetweenpresents", "frametime", "msbetweendisplaychange":
			// The first two are the real frame time; the third is a fallback
			// only used when neither is present.
			if msCol < 0 || !strings.EqualFold(strings.TrimSpace(name), "MsBetweenDisplayChange") {
				msCol = i
			}
		}
	}
	return appCol, pidCol, msCol
}

// Metrics computes FPS statistics for the foreground process, falling back to
// whichever process is presenting the most frames when the foreground window
// is not a rendering application.
func (f *FPSSource) Metrics() *FPSMetrics {
	now := time.Now()
	foreground := foregroundPID()
	self := uint32(os.Getpid())

	f.mu.Lock()
	defer f.mu.Unlock()

	f.pruneLocked(now)

	pid := f.pickProcessLocked(foreground, self, now)
	if pid == 0 {
		return nil
	}
	return f.metricsForLocked(pid, now)
}

// metricsForLocked turns one process's buffered frame times into the numbers
// the HUD shows.
func (f *FPSSource) metricsForLocked(pid uint32, now time.Time) *FPSMetrics {
	frames := f.frames[pid]
	if len(frames) == 0 {
		return nil
	}
	// The newest frame has to be recent, or this process is not presenting any
	// more — a paused game, a closed one, or a window that went to the tray.
	if now.Sub(frames[len(frames)-1].at) > staleAfter {
		return nil
	}

	all := make([]float64, 0, len(frames))
	var sumAll float64
	for _, fr := range frames {
		all = append(all, fr.ms)
		sumAll += fr.ms
	}

	m := &FPSMetrics{
		Process: f.names[pid],
		Cur:     round1(recentRate(frames, curWindow)),
		FrameMs: round1(recentPeak(frames, frameGraphWindow)),
	}
	m.Avg = round1(1000 / (sumAll / float64(len(all))))

	sort.Float64s(all) // ascending, so the worst frames are at the end
	if worst := meanTail(all, 0.01); worst > 0 && len(all) >= minFramesLow1 {
		m.Low1 = round1(1000 / worst)
	}
	if worst := meanTail(all, 0.001); worst > 0 && len(all) >= minFramesLow01 {
		m.Low01 = round1(1000 / worst)
	}
	return m
}

// freshLocked reports whether a process has presented recently enough to still
// be considered running.
func (f *FPSSource) freshLocked(pid uint32, now time.Time) bool {
	frames := f.frames[pid]
	return len(frames) > 0 && now.Sub(frames[len(frames)-1].at) <= staleAfter
}

// recentRate is the frame rate over the last `window` worth of rendered
// frames, walking back from the newest one.
//
// The window is measured in frame times, not in arrival times, and that is the
// whole point. PresentMon writes CSV to a pipe, which the C runtime buffers:
// rows do not trickle out frame by frame, they land in bursts of a few hundred
// at a time. Windowing on arrival left the readout empty between bursts —
// hence a counter that sat at zero and then jumped — even though the frames
// for that moment were merely still in transit.
func recentRate(frames []frameTime, window time.Duration) float64 {
	budget := float64(window.Milliseconds())
	var sum float64
	n := 0
	for i := len(frames) - 1; i >= 0 && sum < budget; i-- {
		sum += frames[i].ms
		n++
	}
	if n == 0 || sum <= 0 {
		return 0
	}
	return 1000 / (sum / float64(n))
}

// recentPeak is the slowest of those same frames — one point of the frame-time
// graph, so that a stutter is visible rather than averaged away.
func recentPeak(frames []frameTime, window time.Duration) float64 {
	budget := float64(window.Milliseconds())
	var sum, peak float64
	for i := len(frames) - 1; i >= 0 && sum < budget; i-- {
		sum += frames[i].ms
		if frames[i].ms > peak {
			peak = frames[i].ms
		}
	}
	return peak
}

// pruneLocked drops frames older than the window and forgets processes that
// have stopped presenting entirely.
func (f *FPSSource) pruneLocked(now time.Time) {
	for pid, frames := range f.frames {
		cut := 0
		for cut < len(frames) && now.Sub(frames[cut].at) > fpsWindow {
			cut++
		}
		frames = frames[cut:]

		if len(frames) == 0 {
			delete(f.frames, pid)
			delete(f.names, pid)
			continue
		}
		f.frames[pid] = frames
	}
}

// pickProcessLocked chooses whose frame rate to report. The foreground window
// wins when it is actually rendering; otherwise the busiest presenter does,
// which covers a game running while its launcher holds focus.
//
// "Actually rendering" means recently, not ever: a paused game keeps its
// frames in the window for a minute, and without the freshness test it would
// go on winning the choice while a second application rendered away unseen.
func (f *FPSSource) pickProcessLocked(foreground, self uint32, now time.Time) uint32 {
	if foreground != 0 && foreground != self &&
		len(f.frames[foreground]) >= minFramesForeground && f.freshLocked(foreground, now) {
		return foreground
	}

	// The desktop compositor and shell always present; they are never the
	// application the user is interested in.
	const minFramesFallback = 30
	best, bestPID := 0, uint32(0)
	for pid, frames := range f.frames {
		name := strings.ToLower(f.names[pid])
		if pid == self || name == "dwm" || name == "explorer" || !f.freshLocked(pid, now) {
			continue
		}
		if len(frames) > best && len(frames) >= minFramesFallback {
			best, bestPID = len(frames), pid
		}
	}
	return bestPID
}

// meanTail averages the worst `frac` share of sorted-ascending frame times,
// which is how 1% and 0.1% lows are conventionally defined.
func meanTail(sortedMs []float64, frac float64) float64 {
	n := int(math.Ceil(float64(len(sortedMs)) * frac))
	if n < 1 {
		n = 1
	}
	if n > len(sortedMs) {
		return 0
	}

	sum := 0.0
	for _, ms := range sortedMs[len(sortedMs)-n:] {
		sum += ms
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
