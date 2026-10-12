//go:build windows

package metrics

import (
	"math"
	"strings"
	"testing"
	"time"
)

// meanTail is what the 1% and 0.1% lows are built on: the mean of the worst
// share of frames, not a percentile of one frame.
func TestMeanTailAveragesTheWorstShare(t *testing.T) {
	// 100 frames: ninety-nine at 10 ms and one at 100 ms.
	frames := make([]float64, 0, 100)
	for i := 0; i < 99; i++ {
		frames = append(frames, 10)
	}
	frames = append(frames, 100)

	if got := meanTail(frames, 0.01); got != 100 {
		t.Errorf("worst 1%% of 100 frames = %v ms, want the single 100 ms frame", got)
	}

	// The worst 10% is that frame plus nine 10 ms ones.
	want := (100 + 9*10.0) / 10
	if got := meanTail(frames, 0.1); math.Abs(got-want) > 1e-9 {
		t.Errorf("worst 10%% = %v ms, want %v", got, want)
	}

	// A share smaller than one frame still has to average something.
	if got := meanTail(frames, 0.0001); got != 100 {
		t.Errorf("sub-frame share = %v ms, want the worst frame", got)
	}
	if got := meanTail(nil, 0.01); got != 0 {
		t.Errorf("empty window = %v, want 0", got)
	}
}

// A low over a handful of frames is not a low — 1% and 0.1% of them are both
// the same single worst frame, which reads as a real measurement in the HUD
// while being pure noise.
func TestLowsAreWithheldUntilTheWindowIsBigEnough(t *testing.T) {
	for _, tc := range []struct {
		name             string
		frames           int
		wantLow1, want01 bool
	}{
		{"a few frames", 20, false, false},
		{"a second of play", 120, true, false},
		{"ten seconds of play", 1200, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &FPSSource{
				frames: map[uint32][]frameTime{},
				names:  map[uint32]string{1: "game"},
			}
			now := time.Now()
			for i := 0; i < tc.frames; i++ {
				f.frames[1] = append(f.frames[1], frameTime{at: now, ms: 10})
			}

			m := f.metricsForLocked(1, now)
			if (m.Low1 > 0) != tc.wantLow1 {
				t.Errorf("%d frames: low1 = %v, reported = %v, want reported = %v", tc.frames, m.Low1, m.Low1 > 0, tc.wantLow1)
			}
			if (m.Low01 > 0) != tc.want01 {
				t.Errorf("%d frames: low01 = %v, reported = %v, want reported = %v", tc.frames, m.Low01, m.Low01 > 0, tc.want01)
			}
		})
	}
}

// Alt-tabbing produces one present that is seconds long. Kept, it is the worst
// frame in the window and drags the lows down for the next minute.
func TestPresentationGapsAreNotFrames(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{},
	}

	var csv strings.Builder
	csv.WriteString("Application,ProcessID,MsBetweenPresents\n")
	for i := 0; i < 200; i++ {
		csv.WriteString("game.exe,1,10\n")
	}
	csv.WriteString("game.exe,1,8000\n") // eight seconds alt-tabbed

	f.consume(strings.NewReader(csv.String()))

	frames := f.frames[1]
	if len(frames) != 200 {
		t.Fatalf("kept %d frames, want the 200 real ones", len(frames))
	}

	m := f.metricsForLocked(1, time.Now())
	if m.Low1 < 99 || m.Low1 > 101 {
		t.Errorf("1%% low = %v fps, want ~100 — the gap leaked into the window", m.Low1)
	}
}

// Frame times are read by column name, because the header differs between
// PresentMon versions and can reappear mid-stream.
func TestHeaderIsResolvedByName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		want   fpsCols
	}{
		{
			"PresentMon 1.x",
			"Application,ProcessID,SwapChainAddress,Runtime,SyncInterval,PresentFlags,Dropped,TimeInSeconds,MsBetweenPresents",
			fpsCols{app: 0, pid: 1, ms: 8, chain: 2, mode: -1},
		},
		{
			"PresentMon 2.x",
			"Application,ProcessID,SwapChainAddress,PresentRuntime,SyncInterval,PresentFlags,AllowsTearing,PresentMode,FrameType,CPUStartTime,FrameTime,CPUBusy",
			fpsCols{app: 0, pid: 1, ms: 10, chain: 2, mode: 7},
		},
		{
			"display change only",
			"Application,ProcessID,MsBetweenDisplayChange",
			fpsCols{app: 0, pid: 1, ms: 2, chain: -1, mode: -1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFPSHeader(tc.header); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A process can present on several swap chains at once, each with its own
// MsBetweenPresents. Mixed together, a 100 fps game and a 50 fps UI chain read
// as about 75 fps — a number neither of them is running at.
func TestFrameRateIsReadFromTheBusiestSwapChain(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{},
	}

	var csv strings.Builder
	csv.WriteString("Application,ProcessID,SwapChainAddress,PresentMode,MsBetweenPresents\n")
	for i := 0; i < 300; i++ {
		csv.WriteString("game.exe,1,0x00000000AAAA0000,Hardware: Independent Flip,10\n")
		if i%2 == 0 {
			csv.WriteString("game.exe,1,0x00000000BBBB0000,Composed: Flip,20\n")
		}
	}
	f.consume(strings.NewReader(csv.String()))

	m := f.metricsForLocked(1, time.Now())
	if m == nil {
		t.Fatal("no metrics")
	}
	if m.Cur != 100 || m.Avg != 100 {
		t.Errorf("cur = %v, avg = %v fps, want 100 — the second chain leaked in", m.Cur, m.Avg)
	}
	if m.Exclusive {
		t.Error("independent flip reported as exclusive fullscreen")
	}
}

// True exclusive fullscreen is the one mode the HUD cannot be drawn over, and
// PresentMon says so per frame — so the UI can tell the user instead of the
// overlay simply not being there.
func TestLegacyFlipIsReportedAsExclusive(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{},
		fso:    map[uint32]fsoState{1: {}}, // checked already: no registry access in a test
	}

	var csv strings.Builder
	csv.WriteString("Application,ProcessID,SwapChainAddress,PresentMode,MsBetweenPresents\n")
	for i := 0; i < 50; i++ {
		csv.WriteString("game.exe,1,0x1,Hardware: Legacy Flip,16.6\n")
	}
	f.consume(strings.NewReader(csv.String()))

	if m := f.metricsForLocked(1, time.Now()); m == nil || !m.Exclusive {
		t.Errorf("got %+v, want exclusive fullscreen reported", m)
	}
}

// PresentMon without rights starts, says "access denied" and exits. That must
// be told apart from a run that worked, or there is never a reason to elevate.
func TestConsumeCountsFrames(t *testing.T) {
	f := &FPSSource{frames: map[uint32][]frameTime{}, names: map[uint32]string{}}
	if n := f.consume(strings.NewReader("")); n != 0 {
		t.Errorf("empty stream: %d frames, want 0", n)
	}
	if n := f.consume(strings.NewReader("Application,ProcessID,MsBetweenPresents\ngame.exe,1,10\ngame.exe,1,NA\n")); n != 1 {
		t.Errorf("got %d frames, want the one parsable row", n)
	}
}

// The frame-time graph must show real frames. Deriving it from the averaged
// current FPS, as the sample stream once did, smooths every stutter away.
func TestFrameMsReportsTheWorstRecentFrame(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{1: "game"},
	}
	now := time.Now()

	// An old hitch, then half a second of smooth frames, then a recent one.
	// Only the recent one belongs to this point of the graph; the old one was
	// drawn seconds ago and is far outside the window walked back over.
	f.frames[1] = append(f.frames[1], frameTime{at: now, ms: 99})
	for i := 0; i < 60; i++ {
		f.frames[1] = append(f.frames[1], frameTime{at: now, ms: 8})
	}
	f.frames[1] = append(f.frames[1], frameTime{at: now, ms: 42})

	if got := f.metricsForLocked(1, now).FrameMs; got != 42 {
		t.Errorf("frameMs = %v, want the recent 42 ms hitch", got)
	}
}

// PresentMon writes to a pipe, and the C runtime buffers it: rows arrive in
// bursts of a few hundred, not one per frame. Windowing on arrival time left
// the readout at zero between bursts and then jumping when one landed, which
// is exactly what the counter looked like in a game.
func TestBurstyDeliveryStillReportsARate(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{1: "game"},
	}
	now := time.Now()

	// One burst of a hundred 10 ms frames, delivered 800 ms ago and nothing
	// since — longer than the half-second the current rate is measured over.
	arrived := now.Add(-800 * time.Millisecond)
	for i := 0; i < 100; i++ {
		f.frames[1] = append(f.frames[1], frameTime{at: arrived, ms: 10})
	}

	if got := f.metricsForLocked(1, now).Cur; got != 100 {
		t.Errorf("cur = %v fps, want 100 — a burst that has stopped arriving is not zero frames", got)
	}
}

// A process that has genuinely stopped presenting reports nothing at all,
// rather than a rate computed from frames that are half a minute old.
func TestStoppedProcessReportsNothing(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{1: "game"},
	}
	now := time.Now()
	for i := 0; i < 100; i++ {
		f.frames[1] = append(f.frames[1], frameTime{at: now.Add(-10 * time.Second), ms: 10})
	}

	if m := f.metricsForLocked(1, now); m != nil {
		t.Errorf("got %+v, want nothing for a process that stopped presenting", m)
	}
}

// Reset clears what it says it clears. The average and the lows restart; the
// current rate and the graph must not blink out, so the last frames survive.
func TestResetKeepsTheLiveReadout(t *testing.T) {
	f := &FPSSource{
		frames: map[uint32][]frameTime{},
		names:  map[uint32]string{1: "game"},
	}
	now := time.Now()
	for i := 0; i < 2000; i++ {
		f.frames[1] = append(f.frames[1], frameTime{at: now, ms: 10})
	}

	before := f.metricsForLocked(1, now)
	if before.Low1 == 0 || before.Low01 == 0 {
		t.Fatalf("expected lows over 2000 frames, got %+v", before)
	}

	f.Reset()

	after := f.metricsForLocked(1, now)
	if after == nil {
		t.Fatal("reset left nothing to report")
	}
	if after.Cur != before.Cur {
		t.Errorf("cur = %v after reset, want it unchanged at %v", after.Cur, before.Cur)
	}
	if after.Low1 != 0 || after.Low01 != 0 {
		t.Errorf("lows = %v / %v after reset, want them withheld until the window refills", after.Low1, after.Low01)
	}
	if n := len(f.frames[1]); n > 100 {
		t.Errorf("kept %d frames after reset, want only the last half second", n)
	}
}
