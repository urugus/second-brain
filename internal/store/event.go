package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) appendEvent(tx *sql.Tx, sessionID *int64, eventType model.EventType, payload string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.Exec(
		`INSERT INTO events (session_id, type, payload, created_at) VALUES (?, ?, ?, ?)`,
		sessionID, string(eventType), payload, now,
	)
	if err != nil {
		return fmt.Errorf("append event %s: %w", eventType, err)
	}
	return nil
}

func (s *Store) ListEventsBySession(sessionID int64) ([]model.Event, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, type, payload, created_at FROM events WHERE session_id = ? ORDER BY created_at ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []model.Event
	for rows.Next() {
		var e model.Event
		var sessionID sql.NullInt64
		var createdAt string
		if err := rows.Scan(&e.ID, &sessionID, &e.Type, &e.Payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if sessionID.Valid {
			e.SessionID = &sessionID.Int64
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			e.CreatedAt = t
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
