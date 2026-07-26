//go:build !windows

package config

// hide is a no-op outside Windows: the dot prefix already hides the folder.
func hide(string) {}
