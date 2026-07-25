//go:build windows

package app

import "testing"

// A position saved while the window sat off-screen (or on a monitor that has
// since been unplugged) must not be reused verbatim — that leaves the overlay
// running but invisible.
func TestClampToScreenRescuesOffScreenPosition(t *testing.T) {
	vx, vy, vw, vh := virtualScreen()
	if vw <= 0 || vh <= 0 {
		t.Skip("no usable screen metrics in this environment")
	}

	const w, h = 320, 560
	// Far to the left of the virtual desktop, whatever its origin is.
	x, y := ClampToScreen(vx-w-1000, vy+40, w, h)

	if x == vx-w-1000 {
		t.Fatal("off-screen x was kept as-is")
	}
	if overlap(x, x+w, vx, vx+vw) < minVisible {
		t.Errorf("clamped x=%d leaves less than %dpx visible horizontally", x, minVisible)
	}
	if overlap(y, y+h, vy, vy+vh) < minVisible {
		t.Errorf("clamped y=%d leaves less than %dpx visible vertically", y, minVisible)
	}
}

// A position that is already on screen must be left untouched, so dragging the
// overlay somewhere deliberate keeps working.
func TestClampToScreenKeepsVisiblePosition(t *testing.T) {
	vx, vy, vw, vh := virtualScreen()
	if vw <= 0 || vh <= 0 {
		t.Skip("no usable screen metrics in this environment")
	}

	wantX, wantY := vx+200, vy+150
	x, y := ClampToScreen(wantX, wantY, 320, 560)
	if x != wantX || y != wantY {
		t.Errorf("ClampToScreen moved an on-screen window: got (%d,%d), want (%d,%d)", x, y, wantX, wantY)
	}
}

// Every corner anchor must land inside the work area.
func TestAnchorPositionsAreOnScreen(t *testing.T) {
	ax, ay, aw, ah := workArea()
	if aw <= 0 || ah <= 0 {
		t.Skip("no usable work area in this environment")
	}

	const w, h = 320, 560
	for _, anchor := range []string{"tl", "tr", "bl", "br"} {
		x, y := AnchorPosition(anchor, w, h)
		if x < ax || x+w > ax+aw {
			t.Errorf("anchor %q: x=%d puts the overlay outside [%d,%d]", anchor, x, ax, ax+aw)
		}
		if y < ay || y+h > ay+ah {
			t.Errorf("anchor %q: y=%d puts the overlay outside [%d,%d]", anchor, y, ay, ay+ah)
		}
	}
}
