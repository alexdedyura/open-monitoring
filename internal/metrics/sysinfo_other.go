//go:build !windows

package metrics

func physicalDisks() []DiskHealthView { return nil }

func isElevated() bool { return false }

func osScale() float64 { return 1 }
