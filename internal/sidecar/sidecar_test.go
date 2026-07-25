package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// A build that bundles a helper must be able to unpack it into a runnable file.
// A build that does not must say so rather than hand back a broken path — the
// callers branch on exactly that.
func TestPathUnpacksOrReportsMissing(t *testing.T) {
	for _, name := range []string{Bridge, PresentMon} {
		t.Run(name, func(t *testing.T) {
			path, err := Path(name)
			if err != nil {
				if _, statErr := embedded.Open("bin/" + name); statErr == nil {
					t.Fatalf("%s is bundled but Path failed: %v", name, err)
				}
				t.Skipf("%s is not bundled into this build", name)
			}

			st, err := os.Stat(path)
			if err != nil {
				t.Fatalf("unpacked path is not usable: %v", err)
			}
			if st.IsDir() || st.Size() == 0 {
				t.Fatalf("unpacked %s is not a runnable file (dir=%v size=%d)", name, st.IsDir(), st.Size())
			}
		})
	}
}

// Unpacking is content-addressed, so asking twice must not rewrite the file or
// hand out a different path — a second app instance relies on that.
func TestPathIsStable(t *testing.T) {
	first, err := Path(Bridge)
	if err != nil {
		t.Skipf("%s is not bundled into this build", Bridge)
	}

	// Drop the memo so the second call redoes the work rather than replaying it.
	mu.Lock()
	delete(resolved, Bridge)
	mu.Unlock()

	second, err := Path(Bridge)
	if err != nil {
		t.Fatalf("second Path call failed: %v", err)
	}
	if first != second {
		t.Errorf("Path is not stable: %q then %q", first, second)
	}
}

// An unknown helper is a programming error, and must not silently produce a
// path that later fails at exec time.
func TestPathRejectsUnknownHelper(t *testing.T) {
	if _, err := Path("no-such-helper.exe"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want fs.ErrNotExist for an unknown helper, got %v", err)
	}
}

// The whole point of embedding the sensor bridge is that it runs on a machine
// with no .NET installed. That cannot be tested here, but "the unpacked file is
// a working executable that emits the expected shape" can.
//
// The helper streams until its parent exits, so this reads a single reading and
// then kills it.
func TestUnpackedBridgeRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns the sensor helper")
	}
	path, err := Path(Bridge)
	if err != nil {
		t.Skipf("%s is not bundled into this build", Bridge)
	}

	// Generous: a compressed single-file publish self-extracts on first run,
	// and enumerating hardware takes a moment on top of that.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "500", strconv.Itoa(os.Getpid()))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("unpacked bridge failed to start: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	if !sc.Scan() {
		t.Fatal("unpacked bridge produced no output")
	}

	// Only the envelope is asserted: which sensors exist depends on the machine.
	var reading struct {
		PawnIO *bool `json:"pawnIo"`
	}
	if err := json.Unmarshal(sc.Bytes(), &reading); err != nil {
		t.Fatalf("bridge output is not JSON: %v (%.120s)", err, sc.Bytes())
	}
	if reading.PawnIO == nil {
		t.Errorf("bridge output is missing the pawnIo field: %.120s", sc.Bytes())
	}
}
