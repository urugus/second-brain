package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

// ImportKBNote creates a note from KB file metadata, using front matter weights
// as initial values instead of computing them from source heuristics.
// Returns the created note ID.
func (s *Store) ImportKBNote(content string, meta model.KBMetadata) (int64, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tagsStr := strings.Join(meta.Tags, ",")

	source := meta.Source
	if source == "" {
		source = "kb-import"
	}

	strength := meta.Strength
	if strength <= 0 {
		strength = 0.30
	}
	salience := meta.Salience
	if salience <= 0 {
		salience = 0.50
	}
	decayRate := meta.DecayRate
	if decayRate <= 0 {
		decayRate = defaultDecayRate
	}

	var consolidatedAtStr *string
	if meta.ConsolidatedAt != nil {
		v := meta.ConsolidatedAt.Format(time.RFC3339)
		consolidatedAtStr = &v
	} else {
		v := nowStr
		consolidatedAtStr = &v
	}

	result, err := s.db.Exec(
		`INSERT INTO notes (session_id, content, tags, source, created_at, updated_at,
		                     strength, decay_rate, salience, recall_count,
		                     last_recalled_at, consolidated_at)
		 VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		content, tagsStr, source, nowStr, nowStr,
		strength, decayRate, salience, meta.RecallCount,
		consolidatedAtStr,
	)
	if err != nil {
		return 0, fmt.Errorf("insert imported note: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// ImportKBEdges creates memory edges from KB metadata related entries.
// kbPathToNoteID maps KB file paths to their imported note IDs.
func (s *Store) ImportKBEdges(noteID int64, related []model.KBRelatedEntry, kbPathToNoteID map[string]int64) error {
	for _, r := range related {
		targetNoteID, ok := kbPathToNoteID[r.Path]
		if !ok || targetNoteID == noteID {
			continue
		}
		weight := r.Weight
		if weight <= 0 {
			weight = 0.10
		}
		if weight > 1 {
			weight = 1
		}
		if err := s.LinkNotes(noteID, targetNoteID, weight, "kb-import"); err != nil {
			continue // best-effort: skip edge errors
		}
	}
	return nil
}
