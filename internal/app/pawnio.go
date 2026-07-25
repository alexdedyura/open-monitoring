package app

import (
	"context"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CPU package temperature and power can only be read from the processor's
// model specific registers, and reading those is a ring-0 operation with no
// user-mode equivalent on Windows. LibreHardwareMonitor reaches them through
// PawnIO, a small signed kernel driver that runs sandboxed modules rather than
// exposing raw MSR access the way the old WinRing0 driver did.
//
// The driver is installed separately, so the app offers to do it rather than
// bundling a driver of its own. Everything here is user-initiated: nothing
// installs without an explicit click.
const (
	pawnIOPackageID = "namazso.PawnIO"
	pawnIOSite      = "https://pawnio.eu"

	// winget downloads and verifies the package, which can take a while on a
	// slow connection; well beyond that it has clearly stalled.
	pawnIOInstallTimeout = 5 * time.Minute
)

// PawnIOInstall reports how an install attempt ended. The driver's presence is
// not part of it: the sensor helper has to be restarted before it can see a
// newly installed driver, so the frontend re-reads the status afterwards.
type PawnIOInstall struct {
	// Started is true when an installer actually ran to completion.
	Started bool `json:"started"`
	// OpenedSite is true when winget was unavailable and the download page was
	// opened instead, leaving the rest to the user.
	OpenedSite bool `json:"openedSite"`
	// Message explains the outcome in a form the UI can show as-is.
	Message string `json:"message"`
}

// InstallPawnIO installs the PawnIO driver with winget, then relaunches the
// sensor helper so the new driver is picked up. When winget is not available
// it opens the official download page instead of fetching an installer itself.
func (a *App) InstallPawnIO() PawnIOInstall {
	winget, err := exec.LookPath("winget")
	if err != nil {
		runtime.BrowserOpenURL(a.ctx, pawnIOSite)
		return PawnIOInstall{
			OpenedSite: true,
			Message:    "winget is not available on this system — the PawnIO download page has been opened instead.",
		}
	}

	ctx, cancel := context.WithTimeout(a.ctx, pawnIOInstallTimeout)
	defer cancel()

	// The package id is fixed, so this never runs anything the user did not ask
	// for. --silent keeps the installer from opening a window behind the app.
	cmd := exec.CommandContext(ctx, winget, "install",
		"--id", pawnIOPackageID,
		"--exact",
		"--silent",
		"--accept-package-agreements",
		"--accept-source-agreements",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runtime.LogErrorf(a.ctx, "pawnio install: %v: %s", err, out)
		return PawnIOInstall{
			Message: "The installer did not finish. You can install PawnIO manually from pawnio.eu.",
		}
	}

	// LibreHardwareMonitor decides once, at startup, whether the driver exists,
	// so the running helper would keep reporting it as missing.
	a.collector.RestartSensors()

	return PawnIOInstall{
		Started: true,
		Message: "PawnIO installed. Reading CPU temperature and power will begin in a few seconds.",
	}
}

// OpenPawnIOSite opens the project's page, for users who would rather install
// it themselves or read what the driver does first.
func (a *App) OpenPawnIOSite() {
	runtime.BrowserOpenURL(a.ctx, pawnIOSite)
}
