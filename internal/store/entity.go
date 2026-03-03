package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

var (
	entityMentionPattern = regexp.MustCompile(`@([A-Za-z][A-Za-z0-9_.-]{1,63})`)
	entityHashtagPattern = regexp.MustCompile(`#([A-Za-z][A-Za-z0-9_.-]{1,63})`)
)

type entityCandidate struct {
	Kind         string
	Name         string
	Normalized   string
	Confidence   float64
	Evidence     string
	SourceNoteID int64
}

type learnedEntity struct {
	ID         int64
	Confidence float64
}

func (s *Store) LearnEntitiesFromNote(note model.Note, source string) error {
	cfg := config.LoadRuntime()
	if !cfg.EntityLearningEnabled {
		return nil
	}

	candidates := extractEntityCandidates(note)
	if len(candidates) == 0 {
		return nil
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	learned := make([]learnedEntity, 0, len(candidates))
	for _, candidate := range candidates {
		entityID, err := upsertEntityFromCandidate(tx, candidate, source, nowStr)
		if err != nil {
			return err
		}
		learned = append(learned, learnedEntity{
			ID:         entityID,
			Confidence: candidate.Confidence,
		})
	}

	if cfg.EntityAutoEdgeMaxPairs > 0 {
		if err := upsertEntityCooccurrenceEdges(tx, learned, note.ID, nowStr, cfg.EntityAutoEdgeMaxPairs); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit entity learning: %w", err)
	}
	return nil
}

func (s *Store) ListEntitiesByNote(noteID int64) ([]model.Entity, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.kind, e.canonical_name, e.normalized_name, e.strength, e.salience, e.status, e.created_at, e.updated_at
		 FROM entities e
		 JOIN note_entities ne ON ne.entity_id = e.id
		 WHERE ne.note_id = ?
		 ORDER BY ne.confidence DESC, e.id ASC`,
		noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("query entities by note: %w", err)
	}
	defer rows.Close()

	var out []model.Entity
	for rows.Next() {
		var (
			entity          model.Entity
			createdAt, upAt string
		)
		if err := rows.Scan(
			&entity.ID,
			&entity.Kind,
			&entity.CanonicalName,
			&entity.NormalizedName,
			&entity.Strength,
			&entity.Salience,
			&entity.Status,
			&createdAt,
			&upAt,
		); err != nil {
			return nil, fmt.Errorf("scan entity by note: %w", err)
		}
		entity.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entity.UpdatedAt, _ = time.Parse(time.RFC3339, upAt)
		out = append(out, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entities by note: %w", err)
	}
	return out, nil
}

func extractEntityCandidates(note model.Note) []entityCandidate {
	candidateByKey := map[string]entityCandidate{}

	for _, rawTag := range note.Tags {
		kind, name, confidence, evidence := parseEntityTag(rawTag)
		addEntityCandidate(candidateByKey, entityCandidate{
			Kind:         kind,
			Name:         name,
			Normalized:   normalizeEntityName(name),
			Confidence:   confidence,
			Evidence:     evidence,
			SourceNoteID: note.ID,
		})
	}

	for _, match := range entityMentionPattern.FindAllStringSubmatch(note.Content, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		addEntityCandidate(candidateByKey, entityCandidate{
			Kind:         "person",
			Name:         name,
			Normalized:   normalizeEntityName(name),
			Confidence:   0.55,
			Evidence:     "content:mention",
			SourceNoteID: note.ID,
		})
	}

	for _, match := range entityHashtagPattern.FindAllStringSubmatch(note.Content, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		addEntityCandidate(candidateByKey, entityCandidate{
			Kind:         "concept",
			Name:         name,
			Normalized:   normalizeEntityName(name),
			Confidence:   0.52,
			Evidence:     "content:hashtag",
			SourceNoteID: note.ID,
		})
	}

	out := make([]entityCandidate, 0, len(candidateByKey))
	for _, candidate := range candidateByKey {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence == out[j].Confidence {
			if out[i].Kind == out[j].Kind {
				return out[i].Normalized < out[j].Normalized
			}
			return out[i].Kind < out[j].Kind
		}
		return out[i].Confidence > out[j].Confidence
	})
	return out
}

func parseEntityTag(tag string) (kind string, name string, confidence float64, evidence string) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", "", 0, ""
	}

	parts := strings.SplitN(tag, ":", 2)
	if len(parts) == 2 {
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		rest := strings.TrimSpace(parts[1])
		switch prefix {
		case "person", "人物":
			return "person", rest, 0.78, "tag:person"
		case "concept", "概念":
			return "concept", rest, 0.72, "tag:concept"
		case "org", "organization", "組織":
			return "org", rest, 0.74, "tag:org"
		case "project", "プロジェクト":
			return "project", rest, 0.74, "tag:project"
		}
	}

	return "concept", tag, 0.60, "tag:generic"
}

func addEntityCandidate(set map[string]entityCandidate, candidate entityCandidate) {
	if candidate.Kind == "" {
		return
	}
	if candidate.Confidence <= 0 {
		return
	}
	if candidate.Normalized == "" {
		return
	}

	key := candidate.Kind + ":" + candidate.Normalized
	if existing, ok := set[key]; ok {
		if candidate.Confidence > existing.Confidence {
			set[key] = candidate
		}
		return
	}
	set[key] = candidate
}

func normalizeEntityName(name string) string {
	return normalizeContextText(name)
}

func upsertEntityFromCandidate(tx *sql.Tx, candidate entityCandidate, source string, nowStr string) (int64, error) {
	canonical := strings.TrimSpace(strings.Join(strings.Fields(candidate.Name), " "))
	if canonical == "" {
		return 0, fmt.Errorf("empty canonical entity name")
	}

	initialStrength := clamp(0.20+(candidate.Confidence*0.30), 0, 1)
	initialSalience := clamp(0.35+(candidate.Confidence*0.45), 0, 1)
	_, err := tx.Exec(
		`INSERT INTO entities
			(kind, canonical_name, normalized_name, strength, salience, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'candidate', ?, ?)
		 ON CONFLICT(kind, normalized_name) DO UPDATE SET
			strength = MIN(1.0, entities.strength + (excluded.strength * (1.0 - entities.strength))),
			salience = MIN(1.0, entities.salience + (excluded.salience * (1.0 - entities.salience))),
			updated_at = excluded.updated_at`,
		candidate.Kind,
		canonical,
		candidate.Normalized,
		initialStrength,
		initialSalience,
		nowStr,
		nowStr,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert entity: %w", err)
	}

	var entityID int64
	if err := tx.QueryRow(
		`SELECT id FROM entities WHERE kind = ? AND normalized_name = ?`,
		candidate.Kind,
		candidate.Normalized,
	).Scan(&entityID); err != nil {
		return 0, fmt.Errorf("get entity id after upsert: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO entity_aliases
			(entity_id, alias, normalized_alias, confidence, source_note_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(entity_id, normalized_alias) DO UPDATE SET
			confidence = MAX(entity_aliases.confidence, excluded.confidence),
			source_note_id = COALESCE(excluded.source_note_id, entity_aliases.source_note_id)`,
		entityID,
		canonical,
		candidate.Normalized,
		candidate.Confidence,
		candidate.SourceNoteID,
		nowStr,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert entity alias: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO note_entities
			(note_id, entity_id, confidence, evidence, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(note_id, entity_id) DO UPDATE SET
			confidence = MAX(note_entities.confidence, excluded.confidence),
			evidence = excluded.evidence,
			source = excluded.source,
			updated_at = excluded.updated_at`,
		candidate.SourceNoteID,
		entityID,
		candidate.Confidence,
		candidate.Evidence,
		source,
		nowStr,
		nowStr,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert note entity relation: %w", err)
	}

	return entityID, nil
}

func upsertEntityCooccurrenceEdges(tx *sql.Tx, entities []learnedEntity, noteID int64, nowStr string, maxPairs int) error {
	if len(entities) < 2 {
		return nil
	}

	byID := make(map[int64]learnedEntity, len(entities))
	for _, entity := range entities {
		if existing, ok := byID[entity.ID]; ok {
			if entity.Confidence > existing.Confidence {
				byID[entity.ID] = entity
			}
			continue
		}
		byID[entity.ID] = entity
	}

	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	linkedPairs := 0
	for i := 0; i < len(ids); i++ {
		if linkedPairs >= maxPairs {
			break
		}
		for j := i + 1; j < len(ids); j++ {
			if linkedPairs >= maxPairs {
				break
			}

			a := byID[ids[i]]
			b := byID[ids[j]]
			avgConfidence := clamp((a.Confidence+b.Confidence)/2, 0, 1)
			weight := clamp(0.08+(0.20*avgConfidence), minEdgeWeightEpsilon, 1)
			evidence := fmt.Sprintf("cooccurs:note#%d", noteID)

			if err := upsertEntityEdge(tx, a.ID, b.ID, "cooccurs", weight, evidence, noteID, nowStr); err != nil {
				return err
			}
			if err := upsertEntityEdge(tx, b.ID, a.ID, "cooccurs", weight, evidence, noteID, nowStr); err != nil {
				return err
			}
			linkedPairs++
		}
	}

	return nil
}

func upsertEntityEdge(tx *sql.Tx, fromID, toID int64, relationType string, weight float64, evidence string, sourceNoteID int64, nowStr string) error {
	_, err := tx.Exec(
		`INSERT INTO entity_edges
			(from_entity_id, to_entity_id, relation_type, weight, evidence, source_note_id, reinforced_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		 ON CONFLICT(from_entity_id, to_entity_id, relation_type) DO UPDATE SET
			weight = MIN(1.0, entity_edges.weight + (excluded.weight * (1.0 - entity_edges.weight))),
			evidence = excluded.evidence,
			source_note_id = excluded.source_note_id,
			reinforced_count = entity_edges.reinforced_count + 1,
			updated_at = excluded.updated_at`,
		fromID,
		toID,
		relationType,
		weight,
		evidence,
		sourceNoteID,
		nowStr,
		nowStr,
	)
	if err != nil {
		return fmt.Errorf("upsert entity edge %d->%d: %w", fromID, toID, err)
	}
	return nil
}
