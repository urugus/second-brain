package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) CreateSyncLog(agent, promptUsed string) (*model.SyncLog, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	result, err := s.db.Exec(
		`INSERT INTO sync_log (agent, prompt_used, status, created_at) VALUES (?, ?, 'pending', ?)`,
		agent, promptUsed, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert sync log: %w", err)
	}

	id, _ := result.LastInsertId()
	return &model.SyncLog{
		ID:         id,
		Agent:      agent,
		PromptUsed: promptUsed,
		Status:     model.SyncPending,
		CreatedAt:  now,
	}, nil
}

func (s *Store) UpdateSyncLog(id int64, status model.SyncStatus, summary string, notesAdded, tasksAdded int, kbFiles string, durationMs int64, errMsg string) error {
	result, err := s.db.Exec(
		`UPDATE sync_log SET status = ?, output_summary = ?, notes_added = ?, tasks_added = ?, kb_files_updated = ?, duration_ms = ?, error_message = ? WHERE id = ?`,
		string(status), summary, notesAdded, tasksAdded, kbFiles, durationMs, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("update sync log: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sync log %d not found", id)
	}
	return nil
}

func (s *Store) LatestSyncLog() (*model.SyncLog, error) {
	row := s.db.QueryRow(
		`SELECT id, agent, prompt_used, output_summary, notes_added, tasks_added, kb_files_updated, duration_ms, status, error_message, created_at
		 FROM sync_log ORDER BY id DESC LIMIT 1`,
	)
	return scanSyncLog(row)
}

func (s *Store) ListSyncLogs(limit int) ([]model.SyncLog, error) {
	rows, err := s.db.Query(
		`SELECT id, agent, prompt_used, output_summary, notes_added, tasks_added, kb_files_updated, duration_ms, status, error_message, created_at
		 FROM sync_log ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query sync logs: %w", err)
	}
	defer rows.Close()

	var logs []model.SyncLog
	for rows.Next() {
		sl, err := scanSyncLogFromRows(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, *sl)
	}
	return logs, rows.Err()
}

func scanSyncLog(row *sql.Row) (*model.SyncLog, error) {
	var sl model.SyncLog
	var status, createdAt string

	err := row.Scan(&sl.ID, &sl.Agent, &sl.PromptUsed, &sl.OutputSummary,
		&sl.NotesAdded, &sl.TasksAdded, &sl.KBFilesUpdated, &sl.DurationMs,
		&status, &sl.ErrorMessage, &createdAt)
	if err != nil {
		return nil, err
	}

	sl.Status = model.SyncStatus(status)
	sl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &sl, nil
}

func scanSyncLogFromRows(rows *sql.Rows) (*model.SyncLog, error) {
	var sl model.SyncLog
	var status, createdAt string

	err := rows.Scan(&sl.ID, &sl.Agent, &sl.PromptUsed, &sl.OutputSummary,
		&sl.NotesAdded, &sl.TasksAdded, &sl.KBFilesUpdated, &sl.DurationMs,
		&status, &sl.ErrorMessage, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan sync log: %w", err)
	}

	sl.Status = model.SyncStatus(status)
	sl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &sl, nil
}
