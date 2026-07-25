//go:build !windows

package app

func startTopmostKeeper() {}
func stopTopmostKeeper()  {}

// ClampToScreen is a no-op outside Windows; the position is used as saved.
func ClampToScreen(x, y, w, h int) (int, int) { return x, y }

// AnchorPosition falls back to a fixed offset outside Windows.
func AnchorPosition(anchor string, w, h int) (int, int) { return 40, 40 }
