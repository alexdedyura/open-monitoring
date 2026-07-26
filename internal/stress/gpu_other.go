//go:build !windows

package stress

import (
	"context"
	"errors"
)

// The GPU job goes through the OpenCL ICD loader that Windows GPU drivers
// install; elsewhere this file only keeps the package building.
func runGPU(context.Context, Options, *reporter) error {
	return errors.New("GPU stress is only available on Windows")
}
