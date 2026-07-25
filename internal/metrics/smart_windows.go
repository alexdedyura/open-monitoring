//go:build windows

package metrics

import (
	"github.com/yusufpapurcu/wmi"
)

// Driver-free SMART.
//
// LibreHardwareMonitor reads SMART through the WinRing0 kernel driver, which
// Microsoft Defender quarantines as a vulnerable driver (CVE-2020-14979).
// Windows exposes the same reliability counters through the storage WMI
// namespace instead — no kernel driver, just elevation — so that is what we
// use for drive temperature and wear.
type msftStorageReliabilityCounter struct {
	DeviceId     string
	Temperature  *uint16
	Wear         *uint16 // percentage of rated life used
	PowerOnHours *uint32
}

type storageReliability struct {
	TempC        float64
	LifePercent  float64 // remaining, 0 = unknown
	PowerOnHours float64
}

// readReliability queries SMART-derived counters for every physical disk. It
// returns them keyed by DeviceId (which matches MSFT_PhysicalDisk.DeviceId) and
// also in query order, so callers can fall back to positional matching if a
// controller reports ids that do not line up. Requires elevation.
func readReliability() (map[string]storageReliability, []storageReliability) {
	var rows []msftStorageReliabilityCounter
	q := "SELECT DeviceId, Temperature, Wear, PowerOnHours FROM MSFT_StorageReliabilityCounter"

	var err error
	runWMI(func() {
		err = wmi.QueryNamespace(q, &rows, `root\microsoft\windows\storage`)
	})
	if err != nil {
		return nil, nil
	}

	byID := make(map[string]storageReliability, len(rows))
	ordered := make([]storageReliability, 0, len(rows))
	for _, r := range rows {
		var sr storageReliability
		if r.Temperature != nil {
			sr.TempC = float64(*r.Temperature)
		}
		if r.Wear != nil && *r.Wear <= 100 {
			sr.LifePercent = float64(100 - *r.Wear)
		}
		if r.PowerOnHours != nil {
			sr.PowerOnHours = float64(*r.PowerOnHours)
		}
		byID[r.DeviceId] = sr
		ordered = append(ordered, sr)
	}
	return byID, ordered
}
