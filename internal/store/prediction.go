package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

const (
	defaultPredictionWindow = 5
)

func (s *Store) EstimateSyncPrediction(limit int) (predictedNotes float64, predictedTasks float64, err error) {
	if limit <= 0 {
		limit = defaultPredictionWindow
	}

	row := s.db.QueryRow(
		`SELECT COALESCE(AVG(notes_added), 0), COALESCE(AVG(tasks_added), 0)
		 FROM (
		   SELECT notes_added, tasks_added
		   FROM sync_log
		   WHERE status = ?
		   ORDER BY id DESC
		   LIMIT ?
		 )`,
		string(model.SyncCompleted), limit,
	)
	if err := row.Scan(&predictedNotes, &predictedTasks); err != nil {
		return 0, 0, fmt.Errorf("estimate sync prediction: %w", err)
	}
	return predictedNotes, predictedTasks, nil
}

func (s *Store) RecordPredictionError(source model.PredictionSource, metric string, predicted, actual float64, priorityDelta int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO prediction_error_log (
			source, metric, predicted_value, actual_value, error_value, priority_delta, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(source), metric, predicted, actual, actual-predicted, priorityDelta, now,
	)
	if err != nil {
		return fmt.Errorf("insert prediction error log: %w", err)
	}
	return nil
}

func (s *Store) ListPredictionErrors(limit int) ([]model.PredictionErrorLog, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(
		`SELECT id, source, metric, predicted_value, actual_value, error_value, priority_delta, created_at
		 FROM prediction_error_log
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query prediction error logs: %w", err)
	}
	defer rows.Close()

	var logs []model.PredictionErrorLog
	for rows.Next() {
		log, err := scanPredictionErrorLogFromRows(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, *log)
	}
	return logs, rows.Err()
}

func (s *Store) AdjustTodoTaskPriorities(delta int, limit int, contextTerms []string) (int, error) {
	if delta == 0 || limit <= 0 {
		return 0, nil
	}
	maxTaskPriority := config.LoadRuntime().TaskPriorityMax
	normalizedContextTerms := normalizePriorityContextTerms(contextTerms)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		`SELECT id, priority, title, description
		 FROM tasks
		 WHERE status = ?
		 ORDER BY priority DESC, id ASC`,
		string(model.TaskTodo),
	)
	if err != nil {
		return 0, fmt.Errorf("query todo tasks for priority adjustment: %w", err)
	}
	defer rows.Close()

	type taskPriority struct {
		id       int64
		priority int
		title    string
		desc     string
	}
	var tasks []taskPriority
	for rows.Next() {
		var t taskPriority
		if err := rows.Scan(&t.id, &t.priority, &t.title, &t.desc); err != nil {
			return 0, fmt.Errorf("scan task for priority adjustment: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate tasks for priority adjustment: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	adjusted := 0
	for _, t := range tasks {
		if len(normalizedContextTerms) > 0 && !taskMatchesPriorityContext(t.title, t.desc, normalizedContextTerms) {
			continue
		}
		next := clampInt(t.priority+delta, 0, maxTaskPriority)
		if next == t.priority {
			continue
		}
		if _, err := tx.Exec(
			`UPDATE tasks SET priority = ?, updated_at = ? WHERE id = ?`,
			next, now, t.id,
		); err != nil {
			return 0, fmt.Errorf("update task priority %d: %w", t.id, err)
		}
		adjusted++
		if adjusted >= limit {
			break
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit priority adjustment: %w", err)
	}
	return adjusted, nil
}

func normalizePriorityContextTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(terms))
	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		key := strings.ToLower(strings.TrimSpace(term))
		if key == "" {
			continue
		}
		if len(key) < 2 {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

func taskMatchesPriorityContext(title, desc string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(title + " " + desc))
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func scanPredictionErrorLog(row *sql.Row) (*model.PredictionErrorLog, error) {
	var log model.PredictionErrorLog
	var source string
	var createdAt string
	err := row.Scan(
		&log.ID,
		&source,
		&log.Metric,
		&log.PredictedValue,
		&log.ActualValue,
		&log.ErrorValue,
		&log.PriorityDelta,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}
	log.Source = model.PredictionSource(source)
	log.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &log, nil
}

func scanPredictionErrorLogFromRows(rows *sql.Rows) (*model.PredictionErrorLog, error) {
	var log model.PredictionErrorLog
	var source string
	var createdAt string
	err := rows.Scan(
		&log.ID,
		&source,
		&log.Metric,
		&log.PredictedValue,
		&log.ActualValue,
		&log.ErrorValue,
		&log.PriorityDelta,
		&createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan prediction error log: %w", err)
	}
	log.Source = model.PredictionSource(source)
	log.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &log, nil
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
