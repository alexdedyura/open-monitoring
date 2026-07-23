package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LHMSource reads the JSON endpoint of LibreHardwareMonitor's built-in web
// server (Options -> Remote Web Server, default http://localhost:8085).
// It is the open-source way to get CPU temperature / package power on Windows
// without shipping a kernel driver. Entirely optional: when the endpoint is
// unreachable the reading is simply nil.
type LHMSource struct {
	url    string
	mu     sync.Mutex
	latest *LHMReading
	cancel context.CancelFunc
}

type LHMReading struct {
	CPUTemp  float64
	CPUPower float64
	CPUClock float64
	GPUName  string
	GPU      *GPUMetrics // fallback for non-NVIDIA GPUs
	Storage  []StorageHealth
}

type lhmNode struct {
	Text     string    `json:"Text"`
	Value    string    `json:"Value"`
	ImageURL string    `json:"ImageURL"`
	Children []lhmNode `json:"Children"`
}

func StartLHM(url string) *LHMSource {
	if url == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &LHMSource{url: url, cancel: cancel}
	go l.loop(ctx)
	return l
}

func (l *LHMSource) Stop() { l.cancel() }

func (l *LHMSource) Latest() *LHMReading {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.latest
}

func (l *LHMSource) loop(ctx context.Context) {
	client := &http.Client{Timeout: 2 * time.Second}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		r := l.fetch(client)
		l.mu.Lock()
		l.latest = r
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (l *LHMSource) fetch(client *http.Client) *LHMReading {
	resp, err := client.Get(l.url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var root lhmNode
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil
	}
	r := &LHMReading{}
	walkChips(&root, r)
	if r.CPUTemp == 0 && r.CPUPower == 0 && r.GPU == nil && len(r.Storage) == 0 {
		return nil
	}
	return r
}

// walkChips looks for chip nodes by their icon (cpu.png for CPUs; nvidia/ati/
// intel icons for GPUs) and extracts the interesting sensors underneath.
func walkChips(n *lhmNode, r *LHMReading) {
	icon := strings.ToLower(n.ImageURL)
	switch {
	case strings.Contains(icon, "cpu.png"):
		parseCPUChip(n, r)
	case strings.Contains(icon, "nvidia.png"), strings.Contains(icon, "ati.png"), strings.Contains(icon, "amd.png"), strings.Contains(icon, "intel.png"):
		parseGPUChip(n, r)
	case strings.Contains(icon, "hdd.png"):
		parseStorageChip(n, r)
	}
	for i := range n.Children {
		walkChips(&n.Children[i], r)
	}
}

func parseCPUChip(chip *lhmNode, r *LHMReading) {
	for i := range chip.Children {
		group := &chip.Children[i]
		switch group.Text {
		case "Temperatures":
			r.CPUTemp = pickSensor(group, []string{"Package", "Tctl", "Average", "Core Max"})
		case "Powers":
			r.CPUPower = pickSensor(group, []string{"Package", "CPU Package"})
		case "Clocks":
			max := 0.0
			for _, s := range group.Children {
				if strings.Contains(s.Text, "Core") {
					if v := parseLHMValue(s.Value); v > max {
						max = v
					}
				}
			}
			r.CPUClock = max
		}
	}
}

func parseGPUChip(chip *lhmNode, r *LHMReading) {
	if r.GPU != nil {
		return // first GPU wins
	}
	g := &GPUMetrics{}
	for i := range chip.Children {
		group := &chip.Children[i]
		switch group.Text {
		case "Temperatures":
			g.TempC = pickSensor(group, []string{"GPU Core"})
		case "Load":
			g.Usage = pickSensor(group, []string{"GPU Core"})
		case "Powers":
			g.PowerW = pickSensor(group, []string{"GPU Package", "GPU Power"})
		case "Clocks":
			g.CoreMHz = pickSensor(group, []string{"GPU Core"})
			g.MemMHz = pickSensor(group, []string{"GPU Memory"})
		case "Data":
			g.MemUsedMB = pickSensor(group, []string{"GPU Memory Used"})
			g.MemTotalMB = pickSensor(group, []string{"GPU Memory Total"})
		case "Controls", "Fans":
			g.FanPercent = pickSensor(group, []string{"GPU Fan", "Fan"})
		}
	}
	if g.Usage > 0 || g.TempC > 0 {
		r.GPUName = chip.Text
		r.GPU = g
	}
}

func parseStorageChip(chip *lhmNode, r *LHMReading) {
	sh := StorageHealth{Name: chip.Text}
	for i := range chip.Children {
		group := &chip.Children[i]
		switch group.Text {
		case "Temperatures":
			sh.TempC = pickSensor(group, []string{"Temperature"})
		case "Levels":
			if v := pickSensor(group, []string{"Remaining Life"}); v > 0 {
				sh.LifePercent = v
			} else if v := pickSensor(group, []string{"Percentage Used"}); v > 0 {
				sh.LifePercent = 100 - v
			}
		case "Data":
			sh.DataWrittenGB = pickSensor(group, []string{"Data Written"})
		}
	}
	if sh.TempC > 0 || sh.LifePercent > 0 || sh.DataWrittenGB > 0 {
		r.Storage = append(r.Storage, sh)
	}
}

// pickSensor returns the first sensor whose name contains one of the preferred
// substrings; falls back to the first sensor in the group.
func pickSensor(group *lhmNode, prefer []string) float64 {
	for _, p := range prefer {
		for _, s := range group.Children {
			if strings.Contains(s.Text, p) {
				return parseLHMValue(s.Value)
			}
		}
	}
	if len(group.Children) > 0 {
		return parseLHMValue(group.Children[0].Value)
	}
	return 0
}

// parseLHMValue turns strings like "54.3 °C", "4,550.5 MHz" or "61,5 W"
// (locale-dependent separators) into a float.
func parseLHMValue(v string) float64 {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ' '); i > 0 {
		v = v[:i]
	}
	hasDot := strings.Contains(v, ".")
	hasComma := strings.Contains(v, ",")
	switch {
	case hasDot && hasComma:
		v = strings.ReplaceAll(v, ",", "") // comma is a thousands separator
	case hasComma:
		v = strings.ReplaceAll(v, ",", ".") // comma is a decimal separator
	}
	var f float64
	if err := json.Unmarshal([]byte(v), &f); err != nil {
		return 0
	}
	return f
}
