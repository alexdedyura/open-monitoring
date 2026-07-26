//go:build windows

package config

import "golang.org/x/sys/windows"

// hide marks the data folder Hidden so it does not clutter the directory the
// exe sits in — Program Files for an installed copy, anywhere for a portable
// one. Best effort: a folder that stays visible still works.
func hide(path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return
	}
	windows.SetFileAttributes(p, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
}
