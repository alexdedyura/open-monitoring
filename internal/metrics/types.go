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
	FPS   *FPSMetrics   `json:"fps,omitempty"`
}

// FPSMetrics describes the foreground application's frame rate (PresentMon).
type FPSMetrics struct {
	Cur     float64 `json:"cur"`   // over the last second
	Avg     float64 `json:"avg"`   // over the 60s window
	Low1    float64 `json:"low1"`  // 1% low
	Low01   float64 `json:"low01"` // 0.1% low
	Process string  `json:"process"`
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

// StorageHealth is SMART-ish data for one physical drive, from the LHM bridge.
type StorageHealth struct {
	Name          string  `json:"name"`
	TempC         float64 `json:"tempC"`
	LifePercent   float64 `json:"lifePercent"` // remaining life, 0 = unknown
	DataWrittenGB float64 `json:"dataWrittenGb"`
}

// DiskHealthView merges WMI drive info with driver-free SMART counters.
type DiskHealthView struct {
	Model         string  `json:"model"`
	SizeGB        float64 `json:"sizeGb"`
	Media         string  `json:"media"` // SSD / HDD / SCM / ""
	Bus           string  `json:"bus"`   // NVMe / SATA / USB / ...
	Health        string  `json:"health"`
	TempC         float64 `json:"tempC"`
	LifePercent   float64 `json:"lifePercent"`
	PowerOnHours  float64 `json:"powerOnHours"`
	DataWrittenGB float64 `json:"dataWrittenGb"`
}

type RAMInfo struct {
	Modules  int     `json:"modules"`
	ModuleGB float64 `json:"moduleGb"`
	SpeedMT  int     `json:"speedMt"`
	Type     string  `json:"type"`
	Vendor   string  `json:"vendor"`
}

// StaticInfo describes the machine; collected once at startup.
type StaticInfo struct {
	CPUModel     string   `json:"cpuModel"`
	CPUCores     int      `json:"cpuCores"`
	CPUThreads   int      `json:"cpuThreads"`
	RAMTotal     uint64   `json:"ramTotal"`
	RAM          RAMInfo  `json:"ram"`
	GPUName      string   `json:"gpuName"`
	Board        string   `json:"board"`
	Disks        []string `json:"disks"`
	OS           string   `json:"os"`
	LHMConnected bool     `json:"lhmConnected"`
	LHMMode      string   `json:"lhmMode"` // bridge | http | none
	NvidiaSMI    bool     `json:"nvidiaSmi"`
	IsAdmin      bool     `json:"isAdmin"`
}
