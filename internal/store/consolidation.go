package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) CreateConsolidationLog(sessionID int64, agent string) (*model.ConsolidationLog, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := s.db.Exec(
		`INSERT INTO consolidation_log (session_id, agent, status, created_at) VALUES (?, ?, 'pending', ?)`,
		sessionID, agent, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert consolidation log: %w", err)
	}

	id, _ := result.LastInsertId()
	t, _ := time.Parse(time.RFC3339, now)

	return &model.ConsolidationLog{
		ID:        id,
		SessionID: sessionID,
		Agent:     agent,
		Status:    model.ConsolidationPending,
		CreatedAt: t,
	}, nil
}

func (s *Store) UpdateConsolidationLog(id int64, status model.ConsolidationStatus, outputSummary, kbFiles string) error {
	result, err := s.db.Exec(
		`UPDATE consolidation_log SET status = ?, output_summary = ?, kb_files_updated = ? WHERE id = ?`,
		string(status), outputSummary, kbFiles, id,
	)
	if err != nil {
		return fmt.Errorf("update consolidation log: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("consolidation log %d not found", id)
	}
	return nil
}

func (s *Store) GetConsolidationLog(id int64) (*model.ConsolidationLog, error) {
	var cl model.ConsolidationLog
	var status, createdAt string

	err := s.db.QueryRow(
		`SELECT id, session_id, agent, input_summary, output_summary, kb_files_updated, status, created_at FROM consolidation_log WHERE id = ?`, id,
	).Scan(&cl.ID, &cl.SessionID, &cl.Agent, &cl.InputSummary, &cl.OutputSummary, &cl.KBFilesUpdated, &status, &createdAt)
	if err != nil {
		return nil, err
	}

	cl.Status = model.ConsolidationStatus(status)
	cl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &cl, nil
}

func (s *Store) HasBeenConsolidated(sessionID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM consolidation_log WHERE session_id = ? AND status = 'completed'`, sessionID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check consolidation: %w", err)
	}
	return count > 0, nil
}

func (s *Store) LatestUnconsolidatedSession() (*model.Session, error) {
	row := s.db.QueryRow(
		`SELECT s.id, s.title, s.goal, s.status, s.started_at, s.ended_at, s.summary, s.created_at, s.updated_at
		 FROM sessions s
		 WHERE s.status IN ('completed', 'abandoned')
		   AND s.id NOT IN (SELECT session_id FROM consolidation_log WHERE status = 'completed')
		 ORDER BY s.id DESC
		 LIMIT 1`,
	)

	sess, err := s.scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sess, err
}
