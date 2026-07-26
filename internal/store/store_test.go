package store

import (
	"path/filepath"
	"testing"
	"time"

	"open-monitoring/internal/metrics"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// A recorded session must come back exactly as it went in: this is the one
// place the app persists user data.
func TestSessionRoundTrip(t *testing.T) {
	s := openTemp(t)

	started := time.Now().UnixMilli()
	id, err := s.CreateSession("GTA V: benchmark", started, 1000)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	batch := []metrics.Sample{
		{T: started, CPU: metrics.CPUMetrics{Usage: 42.5}},
		{T: started + 1000, CPU: metrics.CPUMetrics{Usage: 97.1}},
	}
	if err := s.AppendSamples(id, batch); err != nil {
		t.Fatalf("append samples: %v", err)
	}
	if err := s.EndSession(id, started+2000); err != nil {
		t.Fatalf("end session: %v", err)
	}

	list, err := s.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(list) != 1 || list[0].Name != "GTA V: benchmark" || list[0].Samples != 2 {
		t.Fatalf("listed %+v, want one session with two samples", list)
	}

	got, err := s.SessionSamples(id)
	if err != nil {
		t.Fatalf("session samples: %v", err)
	}
	if len(got) != 2 || got[0].CPU.Usage != 42.5 || got[1].CPU.Usage != 97.1 {
		t.Fatalf("samples came back changed: %+v", got)
	}

	name, err := s.SessionName(id)
	if err != nil || name != "GTA V: benchmark" {
		t.Fatalf("SessionName = %q, %v", name, err)
	}
}

// Deleting a session must take its samples with it, or the database grows
// forever with orphaned rows nothing can reach.
func TestDeleteSessionRemovesSamples(t *testing.T) {
	s := openTemp(t)

	id, _ := s.CreateSession("doomed", 1000, 1000)
	s.AppendSamples(id, []metrics.Sample{{T: 1000}})

	if err := s.DeleteSession(id); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	list, _ := s.ListSessions()
	if len(list) != 0 {
		t.Errorf("session survived deletion: %+v", list)
	}
	if got, _ := s.SessionSamples(id); len(got) != 0 {
		t.Errorf("%d orphaned samples survived deletion", len(got))
	}
}

// Retention removes only finished sessions older than the cutoff — never the
// one still being recorded, whatever its age.
func TestDeleteSessionsBefore(t *testing.T) {
	s := openTemp(t)

	old, _ := s.CreateSession("old", 1000, 1000)
	s.AppendSamples(old, []metrics.Sample{{T: 1000}})
	s.EndSession(old, 2000)

	fresh, _ := s.CreateSession("fresh", 5000, 1000)
	s.EndSession(fresh, 6000)

	s.CreateSession("recording", 500, 1000) // ancient but unfinished

	if err := s.DeleteSessionsBefore(3000); err != nil {
		t.Fatalf("retention: %v", err)
	}

	list, _ := s.ListSessions()
	byName := map[string]bool{}
	for _, si := range list {
		byName[si.Name] = true
	}
	if byName["old"] {
		t.Error("an old finished session survived retention")
	}
	if !byName["fresh"] || !byName["recording"] {
		t.Errorf("retention deleted the wrong sessions: %+v", list)
	}
	if got, _ := s.SessionSamples(old); len(got) != 0 {
		t.Error("retention left orphaned samples behind")
	}
}
