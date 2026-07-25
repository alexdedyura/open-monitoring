//go:build !windows

package metrics

// CleanupRing0Driver is Windows-only; nothing to do elsewhere.
func CleanupRing0Driver() {}
