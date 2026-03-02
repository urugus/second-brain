package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) ComputeOperationalMetrics(windowDays int) (*model.OperationalMetrics, error) {
	if windowDays <= 0 {
		windowDays = 14
	}

	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour).Format(time.RFC3339)
	metrics := &model.OperationalMetrics{WindowDays: windowDays}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notes WHERE created_at >= ?`, since).Scan(&metrics.NotesTotal); err != nil {
		return nil, fmt.Errorf("count notes for metrics: %w", err)
	}

	if err := s.db.QueryRow(`
		SELECT COALESCE(SUM(cnt - 1), 0)
		FROM (
			SELECT LOWER(TRIM(content)) AS key, COUNT(*) AS cnt
			FROM notes
			WHERE created_at >= ?
			GROUP BY key
			HAVING cnt > 1
		)
	`, since).Scan(&metrics.DuplicateNotes); err != nil {
		return nil, fmt.Errorf("count duplicate notes for metrics: %w", err)
	}
	if metrics.NotesTotal > 0 {
		metrics.DuplicateNoteRate = float64(metrics.DuplicateNotes) / float64(metrics.NotesTotal)
	}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE created_at >= ?`, since).Scan(&metrics.TasksTotal); err != nil {
		return nil, fmt.Errorf("count tasks for metrics: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE created_at >= ? AND status = ?`, since, string(model.TaskDone)).Scan(&metrics.TasksDone); err != nil {
		return nil, fmt.Errorf("count done tasks for metrics: %w", err)
	}
	if metrics.TasksTotal > 0 {
		metrics.UsefulTaskGenerationRate = float64(metrics.TasksDone) / float64(metrics.TasksTotal)
	}

	fileUpdates, err := s.listKBFileUpdatesSince(since)
	if err != nil {
		return nil, err
	}
	metrics.UniqueKBFilesUpdated = len(fileUpdates)
	reworked := 0
	for _, count := range fileUpdates {
		if count > 1 {
			reworked++
		}
	}
	metrics.ReworkedKBFiles = reworked
	if metrics.UniqueKBFilesUpdated > 0 {
		metrics.KBReworkRate = float64(metrics.ReworkedKBFiles) / float64(metrics.UniqueKBFilesUpdated)
	}

	return metrics, nil
}

func (s *Store) listKBFileUpdatesSince(since string) (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT kb_files_updated
		FROM sync_log
		WHERE status = ? AND created_at >= ?
		UNION ALL
		SELECT kb_files_updated
		FROM consolidation_log
		WHERE status = ? AND created_at >= ?
	`, string(model.SyncCompleted), since, string(model.ConsolidationCompleted), since)
	if err != nil {
		return nil, fmt.Errorf("query kb updates for metrics: %w", err)
	}
	defer rows.Close()

	fileCounts := map[string]int{}
	for rows.Next() {
		var csv string
		if err := rows.Scan(&csv); err != nil {
			return nil, fmt.Errorf("scan kb updates for metrics: %w", err)
		}
		for _, part := range strings.Split(csv, ",") {
			path := strings.TrimSpace(part)
			if path == "" {
				continue
			}
			fileCounts[path]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate kb updates for metrics: %w", err)
	}
	return fileCounts, nil
}
