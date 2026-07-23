//go:build windows

package metrics

import (
	"strings"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
)

type msftPhysicalDisk struct {
	FriendlyName string
	MediaType    uint16
	BusType      uint16
	HealthStatus uint16
	Size         uint64
}

type win32BaseBoard struct {
	Manufacturer string
	Product      string
}

type win32PhysicalMemory struct {
	Manufacturer     string
	Speed            uint32
	Capacity         uint64
	SMBIOSMemoryType uint16
}

var mediaNames = map[uint16]string{3: "HDD", 4: "SSD", 5: "SCM"}

var busNames = map[uint16]string{
	1: "SCSI", 3: "ATA", 7: "USB", 8: "RAID", 9: "iSCSI",
	10: "SAS", 11: "SATA", 12: "SD", 13: "MMC", 17: "NVMe",
}

var healthNames = map[uint16]string{0: "Healthy", 1: "Warning", 2: "Unhealthy"}

var ddrNames = map[uint16]string{20: "DDR", 21: "DDR2", 24: "DDR3", 26: "DDR4", 34: "DDR5"}

// physicalDisks lists physical drives via the Windows Storage WMI namespace
// (works without elevation, unlike SMART pass-through).
func physicalDisks() []DiskHealthView {
	var disks []msftPhysicalDisk
	q := "SELECT FriendlyName, MediaType, BusType, HealthStatus, Size FROM MSFT_PhysicalDisk"
	if err := wmi.QueryNamespace(q, &disks, `root\microsoft\windows\storage`); err != nil {
		return nil
	}
	out := make([]DiskHealthView, 0, len(disks))
	for _, d := range disks {
		out = append(out, DiskHealthView{
			Model:  strings.TrimSpace(d.FriendlyName),
			SizeGB: float64(d.Size) / 1e9,
			Media:  mediaNames[d.MediaType],
			Bus:    busNames[d.BusType],
			Health: healthNames[d.HealthStatus],
		})
	}
	return out
}

func boardName() string {
	var boards []win32BaseBoard
	if err := wmi.Query("SELECT Manufacturer, Product FROM Win32_BaseBoard", &boards); err != nil || len(boards) == 0 {
		return ""
	}
	return strings.TrimSpace(boards[0].Manufacturer + " " + boards[0].Product)
}

func ramInfo() RAMInfo {
	var mods []win32PhysicalMemory
	if err := wmi.Query("SELECT Manufacturer, Speed, Capacity, SMBIOSMemoryType FROM Win32_PhysicalMemory", &mods); err != nil || len(mods) == 0 {
		return RAMInfo{}
	}
	info := RAMInfo{
		Modules:  len(mods),
		ModuleGB: float64(mods[0].Capacity) / (1 << 30),
		SpeedMT:  int(mods[0].Speed),
		Type:     ddrNames[mods[0].SMBIOSMemoryType],
		Vendor:   strings.TrimSpace(mods[0].Manufacturer),
	}
	return info
}

func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
