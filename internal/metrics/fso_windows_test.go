//go:build windows

package metrics

import (
	"os"
	"testing"
)

// Turning FSO back on must not take the exe's other compatibility settings
// with it — "run as administrator" in particular is often why the entry exists.
func TestWithoutFSOFlagKeepsOtherFlags(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"~ DISABLEDXMAXIMIZEDWINDOWEDMODE", ""},
		{"DISABLEDXMAXIMIZEDWINDOWEDMODE", ""},
		{"~ DISABLEDXMAXIMIZEDWINDOWEDMODE RUNASADMIN", "~ RUNASADMIN"},
		{"~ HIGHDPIAWARE disabledxmaximizedwindowedmode", "~ HIGHDPIAWARE"},
		{"~ RUNASADMIN", "~ RUNASADMIN"},
	} {
		if got := withoutFSOFlag(tc.in); got != tc.want {
			t.Errorf("withoutFSOFlag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The test binary is an ordinary process with no compatibility flags of its
// own, so it is the control case: readable, and nothing to report.
func TestCheckFSOOnAnOrdinaryProcess(t *testing.T) {
	if processImage(uint32(os.Getpid())) == "" {
		t.Fatal("could not read the test's own image path")
	}
	if st := checkFSO(uint32(os.Getpid())); st.off == "app" || st.off == "machine" {
		t.Errorf("got %+v for a process with no compatibility flags", st)
	}
}
