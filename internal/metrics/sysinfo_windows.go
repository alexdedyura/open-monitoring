//go:build windows

package metrics

import (
	"strings"
	"unsafe"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
)

type msftPhysicalDisk struct {
	DeviceId     string
	FriendlyName string
	MediaType    uint16
	BusType      uint16
	HealthStatus uint16
	Size         uint64
}

var mediaNames = map[uint16]string{3: "HDD", 4: "SSD", 5: "SCM"}

var busNames = map[uint16]string{
	1: "SCSI", 3: "ATA", 7: "USB", 8: "RAID", 9: "iSCSI",
	10: "SAS", 11: "SATA", 12: "SD", 13: "MMC", 17: "NVMe",
}

var healthNames = map[uint16]string{0: "Healthy", 1: "Warning", 2: "Unhealthy"}

// physicalDisks lists physical drives via the Windows Storage WMI namespace,
// merged with the driver-free SMART counters from the same namespace.
func physicalDisks() []DiskHealthView {
	var disks []msftPhysicalDisk
	q := "SELECT DeviceId, FriendlyName, MediaType, BusType, HealthStatus, Size FROM MSFT_PhysicalDisk"

	var err error
	runWMI(func() {
		err = wmi.QueryNamespace(q, &disks, `root\microsoft\windows\storage`)
	})
	if err != nil {
		return nil
	}

	byID, ordered := readReliability()
	out := make([]DiskHealthView, 0, len(disks))
	matched := 0
	for _, d := range disks {
		view := DiskHealthView{
			Model:  strings.TrimSpace(d.FriendlyName),
			SizeGB: float64(d.Size) / 1e9,
			Media:  mediaNames[d.MediaType],
			Bus:    busNames[d.BusType],
			Health: healthNames[d.HealthStatus],
		}
		if sr, ok := byID[d.DeviceId]; ok {
			view.TempC = sr.TempC
			view.LifePercent = sr.LifePercent
			view.PowerOnHours = sr.PowerOnHours
			matched++
		}
		out = append(out, view)
	}

	// Some controllers report counter ids that do not line up with the disk
	// ids; when nothing matched but the counts agree, pair them by order.
	if matched == 0 && len(ordered) == len(out) {
		for i := range out {
			out[i].TempC = ordered[i].TempC
			out[i].LifePercent = ordered[i].LifePercent
			out[i].PowerOnHours = ordered[i].PowerOnHours
		}
	}
	return out
}

func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

var procGetDpiForSystem = windows.NewLazySystemDLL("user32.dll").NewProc("GetDpiForSystem")

// osScale reports the display scaling Windows is configured for (1.25 for the
// common "125%" setting), so the UI can match every other app on the desktop.
func osScale() float64 {
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

var procGetUserDefaultLocaleName = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("GetUserDefaultLocaleName")

// localeNameMaxLength is LOCALE_NAME_MAX_LENGTH.
const localeNameMaxLength = 85

// osLang reports the user's locale as a BCP-47 tag ("ru-RU"), so the interface
// can follow the language the rest of Windows is in without being asked. The
// *locale* rather than the UI language: a Russian user running an English
// Windows install is the common case here, and the locale is what they set to
// say which country they are in.
func osLang() string {
	buf := make([]uint16, localeNameMaxLength)
	n, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n-1]) // n counts the terminator
}
