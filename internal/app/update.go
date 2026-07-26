package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Self-update against the repository's versioned releases (v0.4.0, ...). The
// rolling "latest" release the CI refreshes on every push has no semver tag,
// so the updater never sees it — an update only appears when a release is
// actually cut, which is exactly when users should move.
const repoSlug = "alexdedyura/open-monitoring"

// UpdateInfo is the result of a check, shaped for the About tab.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes"`
	URL       string `json:"url"` // release page, for reading the notes in full
}

// newUpdater builds the GitHub client. Every release carries two exes and the
// installer's "amd64" reads as an architecture suffix, so the filter pins the
// one asset that is the update payload: the portable executable. Replacing
// just the exe is enough for installed copies too — the installer's other
// artifacts (shortcuts, uninstaller) are version-independent.
func newUpdater() (*selfupdate.Updater, error) {
	return selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{`^open-monitoring\.exe$`},
	})
}

// CheckForUpdate asks GitHub for the newest versioned release. Called by the
// About tab's button and by the daily background check.
func (a *App) CheckForUpdate() (UpdateInfo, error) {
	info := UpdateInfo{Current: a.version}

	up, err := newUpdater()
	if err != nil {
		return info, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, found, err := up.DetectLatest(ctx, selfupdate.ParseSlug(repoSlug))
	if err != nil || !found {
		return info, err
	}

	a.updMu.Lock()
	a.updRelease = release
	a.updMu.Unlock()

	info.Latest = release.Version()
	info.Available = a.version != "" && !release.LessOrEqual(a.version)
	info.Notes = release.ReleaseNotes
	info.URL = release.URL
	return info, nil
}

// ApplyUpdate downloads the new executable and swaps it in place of the
// running one, rolling back on failure. The old copy keeps running until
// RestartApp; nothing is interrupted mid-recording.
func (a *App) ApplyUpdate() error {
	a.updMu.Lock()
	release := a.updRelease
	a.updMu.Unlock()
	if release == nil {
		return errors.New("no update detected yet")
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return err
	}
	up, err := newUpdater()
	if err != nil {
		return err
	}

	// The portable exe is ~55 MB; give slow connections room.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	return up.UpdateTo(ctx, release, exe)
}

// RestartApp launches the executable at our own path — after ApplyUpdate that
// is the new version — and exits. The relaunched copy waits out the handover
// (see main.go) so the hotkeys and the database are free by the time it needs
// them.
func (a *App) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "OPEN_MONITORING_RELAUNCH=1")
	if err := cmd.Start(); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// watchUpdates is the background half: shortly after startup and then daily,
// while the setting allows. It only announces via the "update" event —
// downloading always stays a user action.
func (a *App) watchUpdates() {
	time.Sleep(20 * time.Second)
	for {
		if a.cfg.UpdateCheck {
			if info, err := a.CheckForUpdate(); err == nil && info.Available {
				runtime.EventsEmit(a.ctx, "update", info)
			}
		}
		time.Sleep(24 * time.Hour)
	}
}
