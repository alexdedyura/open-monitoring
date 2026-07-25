//go:build !windows

package metrics

func physicalDisks() []DiskHealthView { return nil }

func boardName() string { return "" }

func ramInfo() RAMInfo { return RAMInfo{} }

func isElevated() bool { return false }

func osScale() float64 { return 1 }
