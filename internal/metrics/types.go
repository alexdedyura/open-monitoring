package metrics

// Sample is one point-in-time snapshot of every metric the app collects. It is
// emitted to the frontend as a Wails event and persisted during recording.
//
// Throughout these types a zero value means "not available" rather than a
// genuine reading of zero — no sensor here legitimately reports 0 while the
// machine is running, and the UI renders zero as an em dash. Sources are
// expected to leave a field alone rather than write a placeholder into it.
type Sample struct {
	T     int64         `json:"t"` // unix milliseconds
	CPU   CPUMetrics    `json:"cpu"`
	Mem   MemMetrics    `json:"mem"`
	GPU   *GPUMetrics   `json:"gpu,omitempty"`
	Disks []DiskMetrics `json:"disks"`
	Net   NetMetrics    `json:"net"`
	FPS   *FPSMetrics   `json:"fps,omitempty"`
}

type CPUMetrics struct {
	Usage    float64   `json:"usage"` // total load across cores, %
	PerCore  []float64 `json:"perCore"`
	TempC    float64   `json:"tempC"`    // package temperature; needs PawnIO
	PowerW   float64   `json:"powerW"`   // package power draw; needs PawnIO
	ClockMHz float64   `json:"clockMhz"` // fastest core
}

type MemMetrics struct {
	Total       uint64  `json:"total"` // bytes
	Used        uint64  `json:"used"`  // bytes
	UsedPercent float64 `json:"usedPercent"`

	// Page file (swap). Total is the file's current size, which Windows grows
	// under memory pressure — charting it is the point, so both are sampled.
	SwapTotal uint64 `json:"swapTotal"` // bytes
	SwapUsed  uint64 `json:"swapUsed"`  // bytes
}

type GPUMetrics struct {
	Usage      float64 `json:"usage"` // %
	MemUsedMB  float64 `json:"memUsedMb"`
	MemTotalMB float64 `json:"memTotalMb"`
	TempC      float64 `json:"tempC"`
	HotspotC   float64 `json:"hotspotC"` // hot spot / memory junction, where exposed
	PowerW     float64 `json:"powerW"`
	CoreMHz    float64 `json:"coreMhz"`
	MemMHz     float64 `json:"memMhz"`
	FanPercent float64 `json:"fanPercent"`
}

type DiskMetrics struct {
	Name        string  `json:"name"` // drive letter, e.g. "C:"
	ReadBps     float64 `json:"readBps"`
	WriteBps    float64 `json:"writeBps"`
	UsedPercent float64 `json:"usedPercent"` // space in use, refreshed every ~30s
}

type NetMetrics struct {
	UpBps   float64 `json:"upBps"`
	DownBps float64 `json:"downBps"`
}

// FPSMetrics describes the foreground application's frame rate, measured by
// Intel PresentMon. The lows are the conventional gaming metric: the mean of
// the worst 1% / 0.1% of frames over the sampling window, expressed as FPS.
type FPSMetrics struct {
	Cur     float64 `json:"cur"`   // over the last second
	Avg     float64 `json:"avg"`   // over the 60s window
	Low1    float64 `json:"low1"`  // 1% low
	Low01   float64 `json:"low01"` // 0.1% low
	Process string  `json:"process"`
}

// DiskHealthView is one physical drive as shown in the storage panel: identity
// from the Windows storage WMI namespace, wear data from its SMART counters.
type DiskHealthView struct {
	Model         string  `json:"model"`
	SizeGB        float64 `json:"sizeGb"`
	Media         string  `json:"media"` // SSD / HDD / SCM / ""
	Bus           string  `json:"bus"`   // NVMe / SATA / USB / ...
	Health        string  `json:"health"`
	TempC         float64 `json:"tempC"`
	LifePercent   float64 `json:"lifePercent"` // remaining life
	PowerOnHours  float64 `json:"powerOnHours"`
	DataWrittenGB float64 `json:"dataWrittenGb"`
}

// RAMInfo describes the installed memory modules, read from SMBIOS by the
// sensor helper. Modules counts populated slots, and the remaining fields
// describe the first of them — mixed kits are rare enough not to model.
type RAMInfo struct {
	Modules  int     `json:"modules"`
	ModuleGB float64 `json:"moduleGb"`
	SpeedMT  int     `json:"speedMt"`
	Type     string  `json:"type"`
	Vendor   string  `json:"vendor"`
}

// BridgeInfo is the machine description that only the sensor helper can
// supply, so unlike the rest of StaticInfo it is not known at startup.
//
// It is copied into StaticInfo rather than embedded in it: Wails' binding
// generator does not descend into embedded structs to find named types, and
// would emit RAMInfo as `any`.
type BridgeInfo struct {
	GPUName string
	Board   string
	RAM     RAMInfo
}

// SourceStatus reports which optional sensor sources are alive. It is cheap to
// compute, so the UI can poll it while helpers finish starting up.
type SourceStatus struct {
	// SensorsOK reports whether the hardware-sensor helper is delivering data.
	SensorsOK bool `json:"sensorsOk"`
	// PawnIO reports whether the kernel driver that unlocks CPU package
	// temperature and power is installed. See README, "CPU temperature".
	PawnIO        bool   `json:"pawnIo"`
	PawnIOVersion string `json:"pawnIoVersion"`
	// FPSOK reports whether PresentMon started; it needs elevation.
	FPSOK bool `json:"fpsOk"`
}

// StaticInfo describes the machine. Most of it is gathered once at startup;
// the GPU/board/memory fields and the embedded SourceStatus are refreshed on
// request, because they depend on the sensor helper having reported at least
// once.
type StaticInfo struct {
	CPUModel   string   `json:"cpuModel"`
	CPUCores   int      `json:"cpuCores"`
	CPUThreads int      `json:"cpuThreads"`
	CPUBaseMHz float64  `json:"cpuBaseMhz"` // rated base clock, from the CPUID brand info
	RAMTotal   uint64   `json:"ramTotal"`
	Disks      []string `json:"disks"`
	OS         string   `json:"os"`
	Hostname   string   `json:"hostname"`
	BootTime   int64    `json:"bootTime"` // unix seconds; the UI derives uptime

	IsAdmin bool    `json:"isAdmin"`
	OSScale float64 `json:"osScale"` // Windows display scaling, e.g. 1.25

	// Supplied by the sensor helper; see ApplyBridgeInfo.
	GPUName string  `json:"gpuName"`
	Board   string  `json:"board"`
	RAM     RAMInfo `json:"ram"`

	SourceStatus
}

// ApplyBridgeInfo copies in the fields the sensor helper supplies, leaving
// what is already known untouched when the helper has not reported yet.
func (s *StaticInfo) ApplyBridgeInfo(b BridgeInfo) {
	if b.GPUName != "" {
		s.GPUName = b.GPUName
	}
	if b.Board != "" {
		s.Board = b.Board
	}
	if b.RAM.Modules > 0 {
		s.RAM = b.RAM
	}
}
