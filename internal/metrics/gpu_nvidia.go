package metrics

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// NvidiaSource streams GPU metrics from a single long-lived `nvidia-smi -lms`
// process instead of spawning one per sample.
type NvidiaSource struct {
	latest atomic.Pointer[GPUMetrics]
	name   string
	mu     sync.Mutex
	cmd    *exec.Cmd
	closed atomic.Bool
}

const nvidiaFields = "utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,clocks.gr,clocks.mem,fan.speed"

// StartNvidia returns nil when nvidia-smi is not available.
func StartNvidia() *NvidiaSource {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	n := &NvidiaSource{}

	nameCmd := exec.Command(path, "-i", "0", "--query-gpu=name", "--format=csv,noheader")
	hideWindow(nameCmd)
	if out, err := nameCmd.Output(); err == nil {
		n.name = strings.TrimSpace(string(out))
	}
	if n.name == "" {
		return nil
	}

	go n.loop(path)
	return n
}

func (n *NvidiaSource) Name() string { return n.name }

// Latest returns the most recent reading, or nil if data is stale (>10s).
func (n *NvidiaSource) Latest() *GPUMetrics {
	return n.latest.Load()
}

func (n *NvidiaSource) Stop() {
	n.closed.Store(true)
	n.mu.Lock()
	if n.cmd != nil && n.cmd.Process != nil {
		n.cmd.Process.Kill()
	}
	n.mu.Unlock()
}

func (n *NvidiaSource) loop(path string) {
	for !n.closed.Load() {
		cmd := exec.Command(path, "-i", "0",
			"--query-gpu="+nvidiaFields,
			"--format=csv,noheader,nounits", "-lms", "1000")
		hideWindow(cmd)
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		n.mu.Lock()
		n.cmd = cmd
		n.mu.Unlock()

		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if g, ok := parseNvidiaLine(sc.Text()); ok {
				n.latest.Store(g)
			}
		}
		cmd.Wait()
		if !n.closed.Load() {
			time.Sleep(5 * time.Second) // driver hiccup; retry
		}
	}
}

func parseNvidiaLine(line string) (*GPUMetrics, bool) {
	parts := strings.Split(line, ",")
	if len(parts) != 8 {
		return nil, false
	}
	f := func(i int) float64 {
		v := strings.TrimSpace(parts[i])
		if v == "" || strings.Contains(v, "N/A") {
			return 0
		}
		x, _ := strconv.ParseFloat(v, 64)
		return x
	}
	return &GPUMetrics{
		Usage:      f(0),
		MemUsedMB:  f(1),
		MemTotalMB: f(2),
		TempC:      f(3),
		PowerW:     f(4),
		CoreMHz:    f(5),
		MemMHz:     f(6),
		FanPercent: f(7),
	}, true
}
