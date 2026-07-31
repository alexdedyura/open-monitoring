//go:build !windows

package app

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func startTopmostKeeper(_ func() (int, int, bool)) {}
func stopTopmostKeeper()                           {}
func applyClickThrough(_ bool)                     {}
func applyOverlayStyles(_ bool)                    {}

// moveWindowTo has no work-area skew to correct for outside Windows.
func moveWindowTo(ctx context.Context, x, y int) { runtime.WindowSetPosition(ctx, x, y) }

// ClampToScreen is a no-op outside Windows; the position is used as saved.
func ClampToScreen(x, y, w, h int) (int, int) { return x, y }

// AnchorPosition falls back to a fixed offset outside Windows.
func AnchorPosition(anchor string, w, h int) (int, int) { return 40, 40 }

// MaxOverlayHeight has no work area to consult outside Windows.
func MaxOverlayHeight() int { return 2000 }
