//go:build !windows

package app

// pushToast is Windows-only; the in-app toast still shows elsewhere.
func pushToast(title, body string) {}
