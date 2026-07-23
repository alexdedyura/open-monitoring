//go:build !windows

package metrics

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
