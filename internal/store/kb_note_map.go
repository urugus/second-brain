package store

import (
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) MapKBNotes(kbPath string, noteIDs []int64) error {
	if len(noteIDs) == 0 {
		return nil
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	for _, noteID := range noteIDs {
		_, err := s.db.Exec(
			`INSERT INTO kb_note_map (kb_path, note_id, created_at)
			 VALUES (?, ?, ?)
			 ON CONFLICT(kb_path, note_id) DO NOTHING`,
			kbPath, noteID, nowStr,
		)
		if err != nil {
			return fmt.Errorf("map kb note (%s, %d): %w", kbPath, noteID, err)
		}
	}
	return nil
}

func (s *Store) RelatedKBFiles(kbPath string, limit int) ([]model.RelatedKBFile, error) {
	if limit <= 0 {
		limit = 5
	}

	rows, err := s.db.Query(
		`WITH source_notes AS (
			SELECT note_id FROM kb_note_map WHERE kb_path = ?
		),
		related_notes AS (
			SELECT me.to_note_id AS note_id, me.weight
			FROM memory_edges me
			WHERE me.from_note_id IN (SELECT note_id FROM source_notes)
			UNION ALL
			SELECT me.from_note_id AS note_id, me.weight
			FROM memory_edges me
			WHERE me.to_note_id IN (SELECT note_id FROM source_notes)
		)
		SELECT knm.kb_path, SUM(rn.weight) AS total_weight
		FROM related_notes rn
		JOIN kb_note_map knm ON knm.note_id = rn.note_id
		WHERE knm.kb_path != ?
		GROUP BY knm.kb_path
		ORDER BY total_weight DESC
		LIMIT ?`,
		kbPath, kbPath, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query related kb files: %w", err)
	}
	defer rows.Close()

	var results []model.RelatedKBFile
	for rows.Next() {
		var r model.RelatedKBFile
		if err := rows.Scan(&r.Path, &r.Weight); err != nil {
			return nil, fmt.Errorf("scan related kb file: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate related kb files: %w", err)
	}
	return results, nil
}
