// Package sidecar ships the helper executables Open Monitoring spawns.
//
// Two sensors are only reachable through a native helper: hardware sensors
// (LibreHardwareMonitor, a C# library) and frame times (Intel PresentMon, a
// C++ ETW consumer). Neither has a Go equivalent.
//
// Rather than ask users to keep loose .exe files next to the app, the helpers
// are compiled into the Go binary with go:embed and unpacked on first use, so
// `wails build` produces a single distributable executable.
//
// The build script (build.ps1) fills bin/ before the Go build runs. When it
// has not — a fresh checkout, or `wails dev` — Path falls back to the helper's
// build output in the source tree, and failing that reports the helper as
// unavailable, which every caller treats as "do without that sensor".
package sidecar

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Helper binaries, staged into bin/ by build.ps1. The `all:` prefix keeps the
// directive valid when the directory holds nothing but .gitkeep, which is the
// case in a fresh checkout.
//
//go:embed all:bin
var embedded embed.FS

// Names of the helpers this package knows how to unpack.
const (
	Bridge     = "lhm-bridge.exe" // hardware sensors (LibreHardwareMonitor)
	PresentMon = "PresentMon.exe" // frame times for FPS
)

// devSearchPaths maps a helper to where its build output sits in the repo, so
// `wails dev` finds it without a full packaging run.
var devSearchPaths = map[string][]string{
	Bridge:     {filepath.Join("tools", "lhm-bridge", "publish", Bridge)},
	PresentMon: {filepath.Join("tools", "presentmon", PresentMon)},
}

var (
	mu       sync.Mutex
	resolved = map[string]string{} // helper name -> usable path on disk
)

// Path returns a runnable path for the named helper, unpacking it on first
// call. It reports an error when the helper was not bundled into this build
// and is not present in the repo tree either — a normal situation that callers
// handle by doing without that sensor.
func Path(name string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := resolved[name]; ok {
		return p, nil
	}

	p, err := locate(name)
	if err != nil {
		return "", err
	}
	resolved[name] = p
	return p, nil
}

// locate resolves a helper, preferring the embedded copy.
//
// The repo fallback is deliberately only reached when nothing is embedded,
// which is exactly the development case (`wails dev` or `go test` in a
// checkout that has not been staged). A packaged build always carries its
// helpers, so it never looks at the working directory — which would otherwise
// mean running an executable from wherever the user happened to launch it.
func locate(name string) (string, error) {
	data, err := embedded.ReadFile("bin/" + name)
	if err == nil {
		return unpack(name, data)
	}

	if p := findInRepo(name); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("helper %s is not bundled in this build: %w", name, err)
}

// findInRepo looks for a helper's build output in the source tree, relative to
// the working directory. Development only; see locate.
func findInRepo(name string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for _, rel := range devSearchPaths[name] {
		p := filepath.Join(wd, rel)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// unpack writes the helper to the user's local app data, keyed by a hash of
// its contents. A new app version therefore lands on a new filename instead of
// trying to overwrite a copy the previous run may still hold open, and stale
// copies are pruned as they are superseded.
func unpack(name string, data []byte) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	target := filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, hex.EncodeToString(sum[:6]), filepath.Ext(name)))

	// Size is enough to detect a truncated half-written copy; the content hash
	// is already baked into the name.
	if st, err := os.Stat(target); err == nil && st.Size() == int64(len(data)) {
		return target, nil
	}

	if err := writeAtomic(target, data); err != nil {
		return "", err
	}
	pruneOlder(dir, stem, filepath.Base(target))
	return target, nil
}

func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "OpenMonitoring", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// writeAtomic avoids leaving a partially written executable behind if the app
// is killed mid-unpack: another instance would then happily try to run it.
func writeAtomic(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".unpack-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}

	err = os.Rename(tmpName, target)
	if errors.Is(err, fs.ErrExist) {
		return nil // another instance unpacked the same content first
	}
	return err
}

// pruneOlder deletes copies of the same helper left by previous app versions.
// A copy still running is locked by Windows and simply stays until next time.
func pruneOlder(dir, stem, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() != keep && strings.HasPrefix(e.Name(), stem+"-") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
