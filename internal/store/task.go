package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

type TaskFilter struct {
	Status    *model.TaskStatus
	SessionID *int64
}

func (s *Store) CreateTask(title, description string, sessionID *int64, priority int) (*model.Task, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO tasks (session_id, title, description, status, priority, created_at, updated_at) VALUES (?, ?, ?, 'todo', ?, ?, ?)`,
		sessionID, title, description, priority, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	id, _ := result.LastInsertId()

	payload, _ := json.Marshal(map[string]any{"task_id": id, "title": title})
	if err := s.appendEvent(tx, sessionID, model.EventTaskCreated, string(payload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &model.Task{
		ID:          id,
		SessionID:   sessionID,
		Title:       title,
		Description: description,
		Status:      model.TaskTodo,
		Priority:    priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) UpdateTaskStatus(id int64, status model.TaskStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get current status and session_id
	var oldStatus string
	var sessionID sql.NullInt64
	err = tx.QueryRow(`SELECT status, session_id FROM tasks WHERE id = ?`, id).Scan(&oldStatus, &sessionID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	_, err = tx.Exec(`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`, string(status), now, id)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{"task_id": id, "from": oldStatus, "to": string(status)})
	var sid *int64
	if sessionID.Valid {
		sid = &sessionID.Int64
	}
	if err := s.appendEvent(tx, sid, model.EventTaskStatusChanged, string(payload)); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) UpdateTask(id int64, title *string, description *string, priority *int) error {
	now := time.Now().UTC().Format(time.RFC3339)

	setClauses := []string{"updated_at = ?"}
	args := []any{now}

	if title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *title)
	}
	if description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *description)
	}
	if priority != nil {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *priority)
	}

	args = append(args, id)
	query := "UPDATE tasks SET "
	for i, c := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += c
	}
	query += " WHERE id = ?"

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	return nil
}

func (s *Store) GetTask(id int64) (*model.Task, error) {
	row := s.db.QueryRow(
		`SELECT id, session_id, title, description, status, priority, created_at, updated_at FROM tasks WHERE id = ?`, id,
	)
	return scanTask(row)
}

func (s *Store) ListTasks(filter TaskFilter) ([]model.Task, error) {
	query := `SELECT id, session_id, title, description, status, priority, created_at, updated_at FROM tasks WHERE 1=1`
	var args []any

	if filter.Status != nil {
		query += ` AND status = ?`
		args = append(args, string(*filter.Status))
	}
	if filter.SessionID != nil {
		query += ` AND session_id = ?`
		args = append(args, *filter.SessionID)
	}
	query += ` ORDER BY id DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
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

func scanTask(row *sql.Row) (*model.Task, error) {
	var t model.Task
	var sessionID sql.NullInt64
	var status string
	var createdAt, updatedAt string

	err := row.Scan(&t.ID, &sessionID, &t.Title, &t.Description, &status, &t.Priority, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	if sessionID.Valid {
		t.SessionID = &sessionID.Int64
	}
	t.Status = model.TaskStatus(status)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &t, nil
}

func scanTaskFromRows(rows *sql.Rows) (*model.Task, error) {
	var t model.Task
	var sessionID sql.NullInt64
	var status string
	var createdAt, updatedAt string

	err := rows.Scan(&t.ID, &sessionID, &t.Title, &t.Description, &status, &t.Priority, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan task: %w", err)
	}

	if sessionID.Valid {
		t.SessionID = &sessionID.Int64
	}
	t.Status = model.TaskStatus(status)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &t, nil
}
