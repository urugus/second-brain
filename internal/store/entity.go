package store

import (
	"database/sql"
	"fmt"
	"math"
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
	if cfg.EntityDerivedEdgeEnabled &&
		cfg.EntityDerivedEdgeWeight > 0 &&
		cfg.EntityDerivedEdgeMaxLinks > 0 {
		if err := upsertEntityDerivedMemoryEdges(tx, learned, note.ID, nowStr, cfg); err != nil {
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

func (s *Store) DecayEntities(now time.Time) (int, error) {
	cfg := config.LoadRuntime()
	if !cfg.EntityDecayEnabled || cfg.EntityDecayRate <= 0 {
		return 0, nil
	}

	now = now.UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, strength, salience, status, updated_at FROM entities`)
	if err != nil {
		return 0, fmt.Errorf("query entities for decay: %w", err)
	}
	defer rows.Close()

	affected := 0
	for rows.Next() {
		var (
			id        int64
			strength  float64
			salience  float64
			status    string
			updatedAt string
		)
		if err := rows.Scan(&id, &strength, &salience, &status, &updatedAt); err != nil {
			return 0, fmt.Errorf("scan entity for decay: %w", err)
		}

		baseTime, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			continue
		}
		dtDays := now.Sub(baseTime.UTC()).Hours() / 24
		if dtDays <= 0 {
			continue
		}

		newStrength := strength * math.Exp(-cfg.EntityDecayRate*dtDays)
		strengthFloor := cfg.EntityMinStrength
		if strengthFloor > strength {
			strengthFloor = strength
		}
		if newStrength < strengthFloor {
			newStrength = strengthFloor
		}

		newSalience := salience * math.Exp(-(cfg.EntityDecayRate*0.70)*dtDays)
		salienceFloor := cfg.EntityMinSalience
		if salienceFloor > salience {
			salienceFloor = salience
		}
		if newSalience < salienceFloor {
			newSalience = salienceFloor
		}

		newStatus := status
		if status == "confirmed" && newStrength < 0.55 {
			newStatus = "candidate"
		}

		if math.Abs(newStrength-strength) < 1e-9 &&
			math.Abs(newSalience-salience) < 1e-9 &&
			newStatus == status {
			continue
		}

		if _, err := tx.Exec(
			`UPDATE entities
			 SET strength = ?, salience = ?, status = ?, updated_at = ?
			 WHERE id = ?`,
			newStrength,
			newSalience,
			newStatus,
			nowStr,
			id,
		); err != nil {
			return 0, fmt.Errorf("update decayed entity %d: %w", id, err)
		}
		affected++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate entities for decay: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit entity decay: %w", err)
	}
	return affected, nil
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

	var supportingNotes int
	if err := tx.QueryRow(
		`SELECT COUNT(DISTINCT note_id) FROM note_entities WHERE entity_id = ?`,
		entityID,
	).Scan(&supportingNotes); err != nil {
		return 0, fmt.Errorf("count supporting notes for entity %d: %w", entityID, err)
	}
	if supportingNotes >= 2 {
		if _, err := tx.Exec(
			`UPDATE entities
			 SET status = 'confirmed', updated_at = ?
			 WHERE id = ? AND status = 'candidate'`,
			nowStr,
			entityID,
		); err != nil {
			return 0, fmt.Errorf("promote entity status: %w", err)
		}
	}

	return entityID, nil
}

func upsertEntityCooccurrenceEdges(tx *sql.Tx, entities []learnedEntity, noteID int64, nowStr string, maxPairs int) error {
	if len(entities) < 2 {
		return nil
	}

	byID := dedupeLearnedEntities(entities)

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

type entityDerivedNoteLink struct {
	NoteID         int64
	SharedEntities int
	Score          float64
}

func upsertEntityDerivedMemoryEdges(tx *sql.Tx, entities []learnedEntity, sourceNoteID int64, nowStr string, cfg config.Runtime) error {
	byEntityID := dedupeLearnedEntities(entities)
	if len(byEntityID) == 0 {
		return nil
	}

	entityIDs := make([]int64, 0, len(byEntityID))
	placeholders := make([]string, 0, len(byEntityID))
	args := make([]any, 0, len(byEntityID)+1)
	for entityID := range byEntityID {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Slice(entityIDs, func(i, j int) bool { return entityIDs[i] < entityIDs[j] })
	for _, entityID := range entityIDs {
		placeholders = append(placeholders, "?")
		args = append(args, entityID)
	}
	args = append(args, sourceNoteID)

	rows, err := tx.Query(
		fmt.Sprintf(
			`SELECT note_id, entity_id, confidence
			 FROM note_entities
			 WHERE entity_id IN (%s) AND note_id <> ?`,
			strings.Join(placeholders, ","),
		),
		args...,
	)
	if err != nil {
		return fmt.Errorf("query entity-derived memory edge candidates: %w", err)
	}
	defer rows.Close()

	type aggregate struct {
		score  float64
		shared map[int64]struct{}
	}
	aggregates := map[int64]*aggregate{}
	for rows.Next() {
		var (
			noteID     int64
			entityID   int64
			confidence float64
		)
		if err := rows.Scan(&noteID, &entityID, &confidence); err != nil {
			return fmt.Errorf("scan entity-derived memory edge candidate: %w", err)
		}

		sourceEntity, ok := byEntityID[entityID]
		if !ok {
			continue
		}
		a, ok := aggregates[noteID]
		if !ok {
			a = &aggregate{shared: map[int64]struct{}{}}
			aggregates[noteID] = a
		}
		if _, seen := a.shared[entityID]; seen {
			continue
		}
		a.shared[entityID] = struct{}{}
		a.score += clamp(sourceEntity.Confidence*confidence, 0, 1)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entity-derived memory edge candidates: %w", err)
	}

	minShared := cfg.EntityDerivedEdgeMinShared
	if minShared <= 0 {
		minShared = 1
	}

	ranked := make([]entityDerivedNoteLink, 0, len(aggregates))
	for noteID, a := range aggregates {
		sharedCount := len(a.shared)
		if sharedCount < minShared {
			continue
		}
		score := clamp(a.score/float64(sharedCount), 0, 1)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, entityDerivedNoteLink{
			NoteID:         noteID,
			SharedEntities: sharedCount,
			Score:          score,
		})
	}
	if len(ranked) == 0 {
		return nil
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].SharedEntities == ranked[j].SharedEntities {
			if ranked[i].Score == ranked[j].Score {
				return ranked[i].NoteID < ranked[j].NoteID
			}
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].SharedEntities > ranked[j].SharedEntities
	})
	if len(ranked) > cfg.EntityDerivedEdgeMaxLinks {
		ranked = ranked[:cfg.EntityDerivedEdgeMaxLinks]
	}

	for _, item := range ranked {
		weight := clamp(cfg.EntityDerivedEdgeWeight*item.Score, minEdgeWeightEpsilon, 1)
		evidence := fmt.Sprintf("auto:entity-shared shared=%d score=%.2f", item.SharedEntities, item.Score)
		if err := upsertMemoryEdgeTx(tx, sourceNoteID, item.NoteID, weight, evidence, nowStr); err != nil {
			return err
		}
		if err := upsertMemoryEdgeTx(tx, item.NoteID, sourceNoteID, weight, evidence, nowStr); err != nil {
			return err
		}
	}

	return nil
}

func dedupeLearnedEntities(entities []learnedEntity) map[int64]learnedEntity {
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
	return byID
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
