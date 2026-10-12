//go:build windows

package metrics

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Exclusive fullscreen is the one mode no window can be drawn over, but since
// Windows 10 most games that ask for it do not actually get it: "fullscreen
// optimisations" (FSO) quietly run them as a flip-model borderless window,
// with the same latency and a desktop compositor that can still put the HUD on
// top. A game only ends up in true exclusive mode when FSO is off — and the
// usual reason it is off is a setting, either the exe's own compatibility
// checkbox or a "gaming tweak" that switches it off for every game. Both are
// detectable, and both are the user's to switch back, which RestoreFSO does.

const (
	layersKey = `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
	// fsoFlag is what "Disable fullscreen optimizations" writes into the exe's
	// compatibility layers.
	fsoFlag = "DISABLEDXMAXIMIZEDWINDOWEDMODE"

	gameConfigKey = `System\GameConfigStore`
)

// fsoState is the cached outcome of checkFSO for one process.
type fsoState struct {
	off string // "", "app", "machine" or "global" — see FPSMetrics.FSOOff
	exe string // set only alongside off
}

// checkFSO works out whether a setting is what keeps a process out of
// fullscreen optimisations. The per-exe checkbox wins over the global tweak:
// it is the narrower of the two and the more likely to be deliberate.
func checkFSO(pid uint32) fsoState {
	exe := processImage(pid)
	if exe != "" {
		if hasLayerFlag(registry.CURRENT_USER, exe) {
			return fsoState{off: "app", exe: exe}
		}
		if hasLayerFlag(registry.LOCAL_MACHINE, exe) {
			return fsoState{off: "machine", exe: exe}
		}
	}
	if fsoDisabledGlobally() {
		return fsoState{off: "global", exe: exe}
	}
	return fsoState{}
}

// processImage is the full path of a process's executable, "" when it cannot
// be read (a protected process, or one that has just exited).
func processImage(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func hasLayerFlag(root registry.Key, exe string) bool {
	k, err := registry.OpenKey(root, layersKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(exe) // value names are case-insensitive
	if err != nil {
		return false
	}
	for _, tok := range strings.Fields(v) {
		if strings.EqualFold(tok, fsoFlag) {
			return true
		}
	}
	return false
}

// fsoDisabledGlobally recognises the GameConfigStore values that tweak guides
// set to turn FSO off for every game. None of them is documented; this is the
// combination those guides write, and an untouched install has none of it.
func fsoDisabledGlobally() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, gameConfigKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	get := func(name string) uint64 {
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return 0
		}
		return v
	}
	return (get("GameDVR_HonorUserFSEBehaviorMode") == 1 && get("GameDVR_FSEBehaviorMode") == 2) ||
		get("GameDVR_FSEBehavior") == 2 ||
		get("GameDVR_DXGIHonorFSEWindowsCompatible") == 1
}

// RestoreFSO switches fullscreen optimisations back on, for one exe ("app")
// or for every game ("global"). Both live under HKCU, so no elevation is
// needed. The game has to be restarted to pick the change up. The machine-wide
// checkbox is under HKLM and is left to the user — it needs administrator
// rights and belongs to every account on the PC.
func RestoreFSO(kind, exe string) error {
	switch kind {
	case "app":
		return clearLayerFlag(exe)
	case "global":
		return clearGlobalFSOTweak()
	}
	return errors.New("unknown fullscreen-optimisation setting: " + kind)
}

// clearLayerFlag removes the one compatibility flag from the exe's layers and
// leaves any others — "run as administrator", a DPI override — as they were.
func clearLayerFlag(exe string) error {
	if exe == "" {
		return errors.New("no executable given")
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, layersKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(exe)
	if err != nil {
		return err
	}
	if rest := withoutFSOFlag(v); rest != "" {
		return k.SetStringValue(exe, rest)
	}
	return k.DeleteValue(exe)
}

// withoutFSOFlag is a layers string with the FSO flag taken out, "" when
// nothing meaningful is left. "~" only marks the entry as written by the
// compatibility tab; on its own it says nothing.
func withoutFSOFlag(v string) string {
	var keep []string
	for _, tok := range strings.Fields(v) {
		if !strings.EqualFold(tok, fsoFlag) {
			keep = append(keep, tok)
		}
	}
	if len(keep) == 0 || (len(keep) == 1 && keep[0] == "~") {
		return ""
	}
	return strings.Join(keep, " ")
}

// clearGlobalFSOTweak writes back the values an untouched install has.
func clearGlobalFSOTweak() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, gameConfigKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for _, name := range []string{
		"GameDVR_FSEBehaviorMode",
		"GameDVR_HonorUserFSEBehaviorMode",
		"GameDVR_DXGIHonorFSEWindowsCompatible",
	} {
		if err := k.SetDWordValue(name, 0); err != nil {
			return err
		}
	}
	if err := k.DeleteValue("GameDVR_FSEBehavior"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
