package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func (s *Store) LinkNotes(fromNoteID, toNoteID int64, weight float64, evidence string) error {
	if fromNoteID == toNoteID {
		return fmt.Errorf("from_note_id and to_note_id must be different")
	}
	if weight <= 0 {
		return fmt.Errorf("weight must be positive")
	}
	if weight > 1 {
		weight = 1
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO memory_edges
			(from_note_id, to_note_id, weight, evidence, reinforced_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)
		 ON CONFLICT(from_note_id, to_note_id) DO UPDATE SET
			weight = MIN(1.0, memory_edges.weight + (excluded.weight * (1.0 - memory_edges.weight))),
			evidence = excluded.evidence,
			reinforced_count = memory_edges.reinforced_count + 1,
			updated_at = excluded.updated_at`,
		fromNoteID, toNoteID, weight, evidence, nowStr, nowStr,
	)
	if err != nil {
		return fmt.Errorf("link notes: %w", err)
	}
	return nil
}

func (s *Store) RelatedNotes(seedNoteID int64, depth, topK int) ([]model.RelatedNote, error) {
	if depth <= 0 {
		depth = 1
	}
	if topK <= 0 {
		topK = 10
	}

	if _, err := s.GetNote(seedNoteID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("seed note %d not found", seedNoteID)
		}
		return nil, fmt.Errorf("get seed note %d: %w", seedNoteID, err)
	}

	frontier := map[int64]float64{seedNoteID: 1.0}
	visited := map[int64]bool{seedNoteID: true}
	scores := make(map[int64]float64)

	for d := 1; d <= depth; d++ {
		nextFrontier := make(map[int64]float64)

		for fromNoteID, baseScore := range frontier {
			rows, err := s.db.Query(
				`SELECT to_note_id, weight FROM memory_edges WHERE from_note_id = ? ORDER BY weight DESC`,
				fromNoteID,
			)
			if err != nil {
				return nil, fmt.Errorf("query related notes: %w", err)
			}

			for rows.Next() {
				var toNoteID int64
				var edgeWeight float64
				if err := rows.Scan(&toNoteID, &edgeWeight); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scan related edge: %w", err)
				}

				propagated := baseScore * edgeWeight
				if propagated <= 0 {
					continue
				}
				if visited[toNoteID] {
					continue
				}
				if current, ok := nextFrontier[toNoteID]; !ok || propagated > current {
					nextFrontier[toNoteID] = propagated
				}
				if toNoteID != seedNoteID {
					scores[toNoteID] += propagated / float64(d)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("iterate related edges: %w", err)
			}
			rows.Close()
		}

		if len(nextFrontier) == 0 {
			break
		}
		for noteID := range nextFrontier {
			visited[noteID] = true
		}
		frontier = nextFrontier
	}

	type scoredID struct {
		ID    int64
		Score float64
	}

	var ranked []scoredID
	for id, score := range scores {
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scoredID{ID: id, Score: score})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].ID < ranked[j].ID
		}
		return ranked[i].Score > ranked[j].Score
	})

	if len(ranked) > topK {
		ranked = ranked[:topK]
	}

	related := make([]model.RelatedNote, 0, len(ranked))
	for _, item := range ranked {
		note, err := s.GetNote(item.ID)
		if err != nil {
			continue
		}
		related = append(related, model.RelatedNote{
			Note:  *note,
			Score: item.Score,
		})
	}
	return related, nil
}
