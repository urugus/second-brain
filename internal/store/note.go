package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

type NoteFilter struct {
	SessionID *int64
	Tag       *string
}

func (s *Store) CreateNote(content string, sessionID *int64, tags []string, source string) (*model.Note, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tagsStr := strings.Join(tags, ",")

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO notes (session_id, content, tags, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, content, tagsStr, source, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}

	id, _ := result.LastInsertId()

	payload, _ := json.Marshal(map[string]any{"note_id": id, "content": content, "tags": tags})
	if err := s.appendEvent(tx, sessionID, model.EventNoteAdded, string(payload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &model.Note{
		ID:        id,
		SessionID: sessionID,
		Content:   content,
		Tags:      tags,
		Source:    source,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Store) GetNote(id int64) (*model.Note, error) {
	row := s.db.QueryRow(
		`SELECT id, session_id, content, tags, source, created_at, updated_at FROM notes WHERE id = ?`, id,
	)
	return scanNote(row)
}

func (s *Store) ListNotes(filter NoteFilter) ([]model.Note, error) {
	query := `SELECT id, session_id, content, tags, source, created_at, updated_at FROM notes WHERE 1=1`
	var args []any

	if filter.SessionID != nil {
		query += ` AND session_id = ?`
		args = append(args, *filter.SessionID)
	}
	if filter.Tag != nil {
		// Match tag in comma-separated list
		query += ` AND (',' || tags || ',' LIKE '%,' || ? || ',%')`
		args = append(args, *filter.Tag)
	}
	query += ` ORDER BY id DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []model.Note
	for rows.Next() {
		n, err := scanNoteFromRows(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, *n)
	}
	return notes, rows.Err()
}

func scanNote(row *sql.Row) (*model.Note, error) {
	var n model.Note
	var sessionID sql.NullInt64
	var tagsStr string
	var createdAt, updatedAt string

	err := row.Scan(&n.ID, &sessionID, &n.Content, &tagsStr, &n.Source, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	if sessionID.Valid {
		n.SessionID = &sessionID.Int64
	}
	if tagsStr != "" {
		n.Tags = strings.Split(tagsStr, ",")
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &n, nil
}

func scanNoteFromRows(rows *sql.Rows) (*model.Note, error) {
	var n model.Note
	var sessionID sql.NullInt64
	var tagsStr string
	var createdAt, updatedAt string

	err := rows.Scan(&n.ID, &sessionID, &n.Content, &tagsStr, &n.Source, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan note: %w", err)
	}

	if sessionID.Valid {
		n.SessionID = &sessionID.Int64
	}
	if tagsStr != "" {
		n.Tags = strings.Split(tagsStr, ",")
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &n, nil
}
