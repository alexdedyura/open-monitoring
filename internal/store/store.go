package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"

	"open-monitoring/internal/metrics"
)

// Store persists recording sessions in SQLite (pure-Go driver, no cgo).
type Store struct {
	db *sql.DB
}

type SessionInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	StartedAt  int64  `json:"startedAt"`
	EndedAt    int64  `json:"endedAt"` // 0 while recording
	IntervalMs int    `json:"intervalMs"`
	Samples    int    `json:"samples"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			ended_at INTEGER,
			interval_ms INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS samples (
			session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			t INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_samples_session ON samples(session_id, t);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateSession(name string, startedAt int64, intervalMs int) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO sessions(name, started_at, interval_ms) VALUES(?, ?, ?)`,
		name, startedAt, intervalMs)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EndSession(id, endedAt int64) error {
	_, err := s.db.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, endedAt, id)
	return err
}

func (s *Store) AppendSamples(sessionID int64, batch []metrics.Sample) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO samples(session_id, t, data) VALUES(?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, smp := range batch {
		data, err := json.Marshal(smp)
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := stmt.Exec(sessionID, smp.T, string(data)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSessions() ([]SessionInfo, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.name, s.started_at, COALESCE(s.ended_at, 0), s.interval_ms,
		       (SELECT COUNT(*) FROM samples WHERE session_id = s.id)
		FROM sessions s ORDER BY s.started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var si SessionInfo
		if err := rows.Scan(&si.ID, &si.Name, &si.StartedAt, &si.EndedAt, &si.IntervalMs, &si.Samples); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *Store) SessionSamples(id int64) ([]metrics.Sample, error) {
	rows, err := s.db.Query(`SELECT data FROM samples WHERE session_id = ? ORDER BY t`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []metrics.Sample{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var smp metrics.Sample
		if err := json.Unmarshal([]byte(data), &smp); err != nil {
			return nil, err
		}
		out = append(out, smp)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSession(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM samples WHERE session_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) SessionName(id int64) (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sessions WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("session %d: %w", id, err)
	}
	return name, nil
}
