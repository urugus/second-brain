package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

type NoteFilter struct {
	SessionID      *int64
	Tag            *string
	Unconsolidated bool
}

const (
	defaultDecayRate = 0.015
	recallAlpha      = 0.25
	minStrength      = 0.05
)

func (s *Store) CreateNote(content string, sessionID *int64, tags []string, source string) (*model.Note, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tagsStr := strings.Join(tags, ",")
	salience := initialSalience(source, tags, sessionID)
	strength := initialStrength(salience)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO notes (session_id, content, tags, source, created_at, updated_at, strength, decay_rate, salience, recall_count, last_recalled_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL)`,
		sessionID, content, tagsStr, source, nowStr, nowStr, strength, defaultDecayRate, salience,
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
		ID:          id,
		SessionID:   sessionID,
		Content:     content,
		Tags:        tags,
		Source:      source,
		Strength:    strength,
		DecayRate:   defaultDecayRate,
		Salience:    salience,
		RecallCount: 0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) GetNote(id int64) (*model.Note, error) {
	row := s.db.QueryRow(
		`SELECT id, session_id, content, tags, source, strength, decay_rate, salience, recall_count, last_recalled_at, created_at, updated_at, consolidated_at FROM notes WHERE id = ?`, id,
	)
	return scanNote(row)
}

func (s *Store) ListNotes(filter NoteFilter) ([]model.Note, error) {
	query := `SELECT id, session_id, content, tags, source, strength, decay_rate, salience, recall_count, last_recalled_at, created_at, updated_at, consolidated_at FROM notes WHERE 1=1`
	var args []any

	if filter.SessionID != nil {
		query += ` AND session_id = ?`
		args = append(args, *filter.SessionID)
	}
	if filter.Tag != nil {
		query += ` AND (',' || tags || ',' LIKE '%,' || ? || ',%')`
		args = append(args, *filter.Tag)
	}
	if filter.Unconsolidated {
		query += ` AND consolidated_at IS NULL`
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

func (s *Store) CountUnconsolidatedNotes() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM notes WHERE consolidated_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unconsolidated notes: %w", err)
	}
	return count, nil
}

func (s *Store) MarkNotesConsolidated(noteIDs []int64) error {
	if len(noteIDs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)

	placeholders := make([]string, len(noteIDs))
	args := make([]any, len(noteIDs)+1)
	args[0] = now
	for i, id := range noteIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf(
		`UPDATE notes SET consolidated_at = ? WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("mark notes consolidated: %w", err)
	}
	return nil
}

// ApplySleepReplayConsolidation updates note strength and consolidated state in one transaction.
// replayWeightByNoteID maps note ID to replay weight in [0, 1].
func (s *Store) ApplySleepReplayConsolidation(replayWeightByNoteID map[int64]float64, now time.Time) error {
	if len(replayWeightByNoteID) == 0 {
		return nil
	}
	replayAlpha := config.LoadRuntime().SleepReplayAlpha

	now = now.UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for noteID, replayWeight := range replayWeightByNoteID {
		replayWeight = clamp(replayWeight, 0, 1)

		var strength, salience float64
		err := tx.QueryRow(
			`SELECT strength, salience FROM notes WHERE id = ?`,
			noteID,
		).Scan(&strength, &salience)
		if err == sql.ErrNoRows {
			return fmt.Errorf("note %d not found", noteID)
		}
		if err != nil {
			return fmt.Errorf("get note %d: %w", noteID, err)
		}

		delta := replayAlpha * replayWeight * salience * (1 - strength)
		newStrength := clamp(strength+delta, 0, 1)

		if _, err := tx.Exec(
			`UPDATE notes SET strength = ?, consolidated_at = ?, updated_at = ? WHERE id = ?`,
			newStrength, nowStr, nowStr, noteID,
		); err != nil {
			return fmt.Errorf("update consolidated note %d: %w", noteID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sleep consolidation updates: %w", err)
	}
	return nil
}

func (s *Store) RecallNote(id int64, now time.Time, _ string) error {
	now = now.UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var strength, salience float64
	var recallCount int
	err = tx.QueryRow(
		`SELECT strength, salience, recall_count FROM notes WHERE id = ?`,
		id,
	).Scan(&strength, &salience, &recallCount)
	if err == sql.ErrNoRows {
		return fmt.Errorf("note %d not found", id)
	}
	if err != nil {
		return fmt.Errorf("get note: %w", err)
	}

	delta := recallAlpha * salience * (1 - strength)
	newStrength := clamp(strength+delta, 0, 1)

	_, err = tx.Exec(
		`UPDATE notes SET strength = ?, recall_count = ?, last_recalled_at = ?, updated_at = ? WHERE id = ?`,
		newStrength, recallCount+1, nowStr, nowStr, id,
	)
	if err != nil {
		return fmt.Errorf("update note recall state: %w", err)
	}

	return tx.Commit()
}

func (s *Store) DecayMemories(now time.Time) (int, error) {
	now = now.UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT
			id,
			strength,
			decay_rate,
			CASE
				WHEN last_recalled_at IS NOT NULL AND last_recalled_at > updated_at THEN last_recalled_at
				ELSE updated_at
			END AS decay_base_at
		FROM notes
	`)
	if err != nil {
		return 0, fmt.Errorf("query notes for decay: %w", err)
	}
	defer rows.Close()

	affected := 0
	for rows.Next() {
		var id int64
		var strength, decayRate float64
		var baseAt string
		if err := rows.Scan(&id, &strength, &decayRate, &baseAt); err != nil {
			return 0, fmt.Errorf("scan note for decay: %w", err)
		}

		baseTime, err := time.Parse(time.RFC3339, baseAt)
		if err != nil {
			continue
		}
		dtDays := now.Sub(baseTime).Hours() / 24
		if dtDays <= 0 {
			continue
		}

		newStrength := strength * math.Exp(-decayRate*dtDays)
		if newStrength < minStrength {
			newStrength = minStrength
		}
		if math.Abs(newStrength-strength) < 1e-9 {
			continue
		}

		if _, err := tx.Exec(
			`UPDATE notes SET strength = ?, updated_at = ? WHERE id = ?`,
			newStrength, nowStr, id,
		); err != nil {
			return 0, fmt.Errorf("update decayed note %d: %w", id, err)
		}
		affected++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate notes for decay: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit decay: %w", err)
	}
	return affected, nil
}

func scanNote(row *sql.Row) (*model.Note, error) {
	var n model.Note
	var sessionID sql.NullInt64
	var tagsStr string
	var lastRecalledAt sql.NullString
	var createdAt, updatedAt string
	var consolidatedAt sql.NullString

	err := row.Scan(
		&n.ID,
		&sessionID,
		&n.Content,
		&tagsStr,
		&n.Source,
		&n.Strength,
		&n.DecayRate,
		&n.Salience,
		&n.RecallCount,
		&lastRecalledAt,
		&createdAt,
		&updatedAt,
		&consolidatedAt,
	)
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
	if lastRecalledAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastRecalledAt.String)
		n.LastRecalledAt = &t
	}
	if consolidatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, consolidatedAt.String)
		n.ConsolidatedAt = &t
	}
	return &n, nil
}

func scanNoteFromRows(rows *sql.Rows) (*model.Note, error) {
	var n model.Note
	var sessionID sql.NullInt64
	var tagsStr string
	var lastRecalledAt sql.NullString
	var createdAt, updatedAt string
	var consolidatedAt sql.NullString

	err := rows.Scan(
		&n.ID,
		&sessionID,
		&n.Content,
		&tagsStr,
		&n.Source,
		&n.Strength,
		&n.DecayRate,
		&n.Salience,
		&n.RecallCount,
		&lastRecalledAt,
		&createdAt,
		&updatedAt,
		&consolidatedAt,
	)
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
	if lastRecalledAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastRecalledAt.String)
		n.LastRecalledAt = &t
	}
	if consolidatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, consolidatedAt.String)
		n.ConsolidatedAt = &t
	}
	return &n, nil
}

func initialSalience(source string, tags []string, sessionID *int64) float64 {
	salience := 0.35
	switch source {
	case "manual":
		salience += 0.15
	case "sync":
		salience += 0.05
	}
	tagBonus := float64(len(tags)) * 0.03
	if tagBonus > 0.20 {
		tagBonus = 0.20
	}
	salience += tagBonus
	if sessionID != nil {
		salience += 0.05
	}
	return clamp(salience, 0, 1)
}

func initialStrength(salience float64) float64 {
	return clamp(0.20+0.50*salience, 0, 1)
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
