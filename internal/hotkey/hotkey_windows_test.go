//go:build windows

package hotkey

import "testing"

func TestParseCombos(t *testing.T) {
	for _, tc := range []struct {
		combo    string
		wantMods uint32
		wantVK   uint32
	}{
		{"Ctrl+Alt+H", modControl | modAlt, 'H'},
		{"ctrl+alt+b", modControl | modAlt, 'B'},
		{"Ctrl+Shift+F12", modControl | modShift, 0x7B},
		{"Alt+F1", modAlt, 0x70},
		{"Win+Shift+7", modWin | modShift, '7'},
		{"F9", 0, 0x78}, // no modifier is legal, if unwise
	} {
		mods, vk, err := parse(tc.combo)
		if err != nil {
			t.Errorf("parse(%q) failed: %v", tc.combo, err)
			continue
		}
		if mods != tc.wantMods || vk != tc.wantVK {
			t.Errorf("parse(%q) = mods %#x, vk %#x; want %#x, %#x", tc.combo, mods, vk, tc.wantMods, tc.wantVK)
		}
	}
}

// A combination that cannot be registered must be reported, not guessed at —
// silently binding something else would be worse than binding nothing.
func TestParseRejectsNonsense(t *testing.T) {
	for _, combo := range []string{"", "Ctrl+", "Hyper+H", "Ctrl+Escape", "Ctrl+F0", "Ctrl+F25", "Ctrl+ ", "Ctrl+Alt+HH"} {
		if _, _, err := parse(combo); err == nil {
			t.Errorf("parse(%q) accepted a combination it cannot register", combo)
		}
	}
}

// The errors line up with the bindings by position, which is what lets Settings
// point at the shortcut that failed. An entry that slid to another index would
// blame the wrong combination — worse than saying nothing.
func TestRegisterErrorsAlignWithBindings(t *testing.T) {
	m, errs := Register([]Binding{
		{Combo: "Ctrl+Nonsense"},
		{Combo: "Ctrl+Alt+Shift+F24"},
	})
	defer m.Stop()

	if len(errs) != 2 {
		t.Fatalf("len(errs) = %d, want one entry per binding", len(errs))
	}
	if errs[0] == nil {
		t.Error("an unparseable combination was reported as live")
	}
	// errs[1] is up to the desktop: only its position is under test here.
}

// Register hands back a manager that Stop can always be called on, including
// when every binding was rejected.
func TestRegisterAndStop(t *testing.T) {
	fired := make(chan struct{}, 1)
	m, errs := Register([]Binding{
		{Combo: "Ctrl+Alt+Shift+F24", Do: func() { fired <- struct{}{} }},
	})
	if m == nil {
		t.Fatal("no manager returned")
	}
	for _, err := range errs {
		t.Logf("not registered on this machine: %v", err)
	}
	m.Stop()

	// Stopping twice, or stopping a manager that never started, must not panic
	// or block — Shutdown calls it unconditionally.
	m.Stop()
	var zero *Manager
	zero.Stop()
}
