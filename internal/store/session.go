package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) CreateSession(title, goal string) (*model.Session, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check no active session exists
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status = 'active'`).Scan(&count); err != nil {
		return nil, fmt.Errorf("check active session: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("an active session already exists; end or abandon it first")
	}

	result, err := tx.Exec(
		`INSERT INTO sessions (title, goal, status, started_at, created_at, updated_at) VALUES (?, ?, 'active', ?, ?, ?)`,
		title, goal, nowStr, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	id, _ := result.LastInsertId()

	payload, _ := json.Marshal(map[string]string{"title": title, "goal": goal})
	if err := s.appendEvent(tx, &id, model.EventSessionStarted, string(payload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &model.Session{
		ID:        id,
		Title:     title,
		Goal:      goal,
		Status:    model.SessionActive,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Store) EndSession(id int64, summary string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE sessions SET status = 'completed', ended_at = ?, summary = ?, updated_at = ? WHERE id = ? AND status = 'active'`,
		now, summary, now, id,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %d is not active", id)
	}

	payload, _ := json.Marshal(map[string]string{"summary": summary})
	if err := s.appendEvent(tx, &id, model.EventSessionEnded, string(payload)); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) AbandonSession(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE sessions SET status = 'abandoned', ended_at = ?, updated_at = ? WHERE id = ? AND status = 'active'`,
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %d is not active", id)
	}

	if err := s.appendEvent(tx, &id, model.EventSessionAbandoned, "{}"); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetSession(id int64) (*model.Session, error) {
	return s.scanSession(s.db.QueryRow(
		`SELECT id, title, goal, status, started_at, ended_at, summary, created_at, updated_at FROM sessions WHERE id = ?`, id,
	))
}

func (s *Store) ActiveSession() (*model.Session, error) {
	sess, err := s.scanSession(s.db.QueryRow(
		`SELECT id, title, goal, status, started_at, ended_at, summary, created_at, updated_at FROM sessions WHERE status = 'active' LIMIT 1`,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sess, err
}

func (s *Store) ListSessions(status *model.SessionStatus) ([]model.Session, error) {
	var query string
	var args []any

	if status != nil {
		query = `SELECT id, title, goal, status, started_at, ended_at, summary, created_at, updated_at FROM sessions WHERE status = ? ORDER BY id DESC`
		args = append(args, string(*status))
	} else {
		query = `SELECT id, title, goal, status, started_at, ended_at, summary, created_at, updated_at FROM sessions ORDER BY id DESC`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		sess, err := s.scanSessionFromRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

func (s *Store) scanSession(row *sql.Row) (*model.Session, error) {
	var sess model.Session
	var status string
	var startedAt, createdAt, updatedAt string
	var endedAt sql.NullString

	err := row.Scan(&sess.ID, &sess.Title, &sess.Goal, &status, &startedAt, &endedAt, &sess.Summary, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	sess.Status = model.SessionStatus(status)
	sess.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		sess.EndedAt = &t
	}

	return &sess, nil
}

func (s *Store) scanSessionFromRows(rows *sql.Rows) (*model.Session, error) {
	var sess model.Session
	var status string
	var startedAt, createdAt, updatedAt string
	var endedAt sql.NullString

	err := rows.Scan(&sess.ID, &sess.Title, &sess.Goal, &status, &startedAt, &endedAt, &sess.Summary, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}

	sess.Status = model.SessionStatus(status)
	sess.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		sess.EndedAt = &t
	}

	return &sess, nil
}
