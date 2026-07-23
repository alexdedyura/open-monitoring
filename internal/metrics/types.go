package metrics

// Sample is one point-in-time snapshot of all collected metrics.
// It is emitted to the frontend as a Wails event and persisted during recording.
type Sample struct {
	T     int64         `json:"t"` // unix milliseconds
	CPU   CPUMetrics    `json:"cpu"`
	Mem   MemMetrics    `json:"mem"`
	GPU   *GPUMetrics   `json:"gpu,omitempty"`
	Disks []DiskMetrics `json:"disks"`
	Net   NetMetrics    `json:"net"`
}

type CPUMetrics struct {
	Usage    float64   `json:"usage"` // total load, %
	PerCore  []float64 `json:"perCore"`
	TempC    float64   `json:"tempC"`    // 0 = unavailable (needs LibreHardwareMonitor)
	PowerW   float64   `json:"powerW"`   // 0 = unavailable
	ClockMHz float64   `json:"clockMhz"` // 0 = unavailable
}

type MemMetrics struct {
	Total       uint64  `json:"total"` // bytes
	Used        uint64  `json:"used"`  // bytes
	UsedPercent float64 `json:"usedPercent"`
}

type GPUMetrics struct {
	Usage      float64 `json:"usage"` // %
	MemUsedMB  float64 `json:"memUsedMb"`
	MemTotalMB float64 `json:"memTotalMb"`
	TempC      float64 `json:"tempC"`
	PowerW     float64 `json:"powerW"`
	CoreMHz    float64 `json:"coreMhz"`
	MemMHz     float64 `json:"memMhz"`
	FanPercent float64 `json:"fanPercent"`
}

type DiskMetrics struct {
	Name        string  `json:"name"` // drive letter, e.g. "C:"
	ReadBps     float64 `json:"readBps"`
	WriteBps    float64 `json:"writeBps"`
	UsedPercent float64 `json:"usedPercent"` // space, refreshed every ~30s
}

type NetMetrics struct {
	UpBps   float64 `json:"upBps"`
	DownBps float64 `json:"downBps"`
}

// StaticInfo describes the machine; collected once at startup.
type StaticInfo struct {
	CPUModel     string   `json:"cpuModel"`
	CPUCores     int      `json:"cpuCores"`
	CPUThreads   int      `json:"cpuThreads"`
	RAMTotal     uint64   `json:"ramTotal"`
	GPUName      string   `json:"gpuName"`
	Disks        []string `json:"disks"`
	OS           string   `json:"os"`
	LHMConnected bool     `json:"lhmConnected"`
	NvidiaSMI    bool     `json:"nvidiaSmi"`
}
