package store

import (
	"fmt"

	"github.com/urugus/second-brain/internal/model"
)

// ListRecentNotesForSyncFocus returns recent notes used as learning signals for sync focus.
func (s *Store) ListRecentNotesForSyncFocus(limit int) ([]model.Note, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT id, session_id, content, tags, source, strength, decay_rate, salience, recall_count, last_recalled_at, created_at, updated_at, consolidated_at
		 FROM notes
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query notes for sync focus: %w", err)
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

// ListActiveTasksForSyncFocus returns active tasks used as current intent signals.
func (s *Store) ListActiveTasksForSyncFocus(limit int) ([]model.Task, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, session_id, title, description, status, priority, created_at, updated_at
		 FROM tasks
		 WHERE status IN (?, ?)
		 ORDER BY priority DESC, id DESC
		 LIMIT ?`,
		string(model.TaskInProgress),
		string(model.TaskTodo),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query tasks for sync focus: %w", err)
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		t, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, rows.Err()
}
