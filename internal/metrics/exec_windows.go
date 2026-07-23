//go:build windows

package metrics

import (
	"os/exec"
	"syscall"
)

// hideWindow prevents console windows from flashing when spawning CLI tools.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
