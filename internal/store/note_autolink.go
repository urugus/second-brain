package store

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/config"
)

type createAutoLinkCandidate struct {
	ID         int64
	SessionID  int64
	HasSession bool
	Content    string
	Tags       []string
	CreatedAt  time.Time
}

type scoredCreateAutoLink struct {
	NoteID    int64
	Score     float64
	TokenJacc float64
	TagJacc   float64
	Context   float64
}

var createAutoLinkStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "by": {},
	"for": {}, "from": {}, "in": {}, "is": {}, "it": {}, "of": {}, "on": {}, "or": {},
	"that": {}, "the": {}, "this": {}, "to": {}, "was": {}, "with": {},
}

func (s *Store) autoLinkNewNote(noteID int64, sessionID *int64, content string, tags []string, createdAt time.Time) {
	cfg := config.LoadRuntime()
	if !cfg.MemoryEdgeCreateEnabled {
		return
	}
	if cfg.MemoryEdgeCreateWeight <= 0 || cfg.MemoryEdgeCreateMaxLinks <= 0 || cfg.MemoryEdgeCreateCandidates <= 0 {
		return
	}

	noteTokens := createAutoLinkTokens(content)
	noteTagSet := normalizedTagSet(tags)
	if len(noteTokens) == 0 && len(noteTagSet) == 0 {
		return
	}

	candidates, err := s.listCreateAutoLinkCandidates(noteID, cfg.MemoryEdgeCreateCandidates)
	if err != nil || len(candidates) == 0 {
		return
	}

	scored := make([]scoredCreateAutoLink, 0, len(candidates))
	for _, candidate := range candidates {
		score := scoreCreateAutoLinkCandidate(sessionID, createdAt, noteTokens, noteTagSet, candidate)
		if score.Score < cfg.MemoryEdgeCreateMinScore {
			continue
		}
		scored = append(scored, score)
	}

	if len(scored) == 0 {
		return
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].NoteID < scored[j].NoteID
		}
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > cfg.MemoryEdgeCreateMaxLinks {
		scored = scored[:cfg.MemoryEdgeCreateMaxLinks]
	}

	for _, item := range scored {
		weight := clamp(cfg.MemoryEdgeCreateWeight*item.Score, 0, 1)
		if weight <= 0 {
			continue
		}
		evidence := fmt.Sprintf(
			"auto:create-similarity score=%.2f token=%.2f tag=%.2f ctx=%.2f",
			item.Score,
			item.TokenJacc,
			item.TagJacc,
			item.Context,
		)
		if err := s.LinkNotes(noteID, item.NoteID, weight, evidence); err != nil {
			continue
		}
		_ = s.LinkNotes(item.NoteID, noteID, weight, evidence)
	}
}

func (s *Store) listCreateAutoLinkCandidates(noteID int64, limit int) ([]createAutoLinkCandidate, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, content, tags, created_at
		 FROM notes
		 WHERE id <> ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		noteID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query create autolink candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]createAutoLinkCandidate, 0, limit)
	for rows.Next() {
		var (
			candidate createAutoLinkCandidate
			session   sql.NullInt64
			tagsStr   string
			createdAt string
		)
		if err := rows.Scan(&candidate.ID, &session, &candidate.Content, &tagsStr, &createdAt); err != nil {
			return nil, fmt.Errorf("scan create autolink candidate: %w", err)
		}
		if session.Valid {
			candidate.HasSession = true
			candidate.SessionID = session.Int64
		}
		if tagsStr != "" {
			candidate.Tags = strings.Split(tagsStr, ",")
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			continue
		}
		candidate.CreatedAt = t.UTC()
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate create autolink candidates: %w", err)
	}
	return candidates, nil
}

func scoreCreateAutoLinkCandidate(
	noteSessionID *int64,
	noteCreatedAt time.Time,
	noteTokens []string,
	noteTagSet map[string]struct{},
	candidate createAutoLinkCandidate,
) scoredCreateAutoLink {
	candidateTokens := createAutoLinkTokens(candidate.Content)
	candidateTagSet := normalizedTagSet(candidate.Tags)

	tokenJacc := jaccardScore(noteTokens, candidateTokens)
	tagJacc := jaccardScoreFromSets(noteTagSet, candidateTagSet)
	if tokenJacc == 0 && tagJacc == 0 {
		return scoredCreateAutoLink{NoteID: candidate.ID}
	}

	context := createAutoLinkContextScore(noteSessionID, noteCreatedAt, candidate.HasSession, candidate.SessionID, candidate.CreatedAt)
	score := clamp((tokenJacc*0.50)+(tagJacc*0.30)+(context*0.20), 0, 1)

	return scoredCreateAutoLink{
		NoteID:    candidate.ID,
		Score:     score,
		TokenJacc: tokenJacc,
		TagJacc:   tagJacc,
		Context:   context,
	}
}

func createAutoLinkContextScore(noteSessionID *int64, noteCreatedAt time.Time, candidateHasSession bool, candidateSessionID int64, candidateCreatedAt time.Time) float64 {
	sessionScore := 0.0
	if noteSessionID != nil && candidateHasSession && *noteSessionID == candidateSessionID {
		sessionScore = 1.0
	}

	hours := math.Abs(noteCreatedAt.Sub(candidateCreatedAt).Hours())
	recencyScore := 0.0
	switch {
	case hours <= 2:
		recencyScore = 1.0
	case hours <= 24:
		recencyScore = 0.70
	case hours <= 72:
		recencyScore = 0.40
	case hours <= 168:
		recencyScore = 0.20
	}

	if recencyScore > sessionScore {
		return recencyScore
	}
	return sessionScore
}

func createAutoLinkTokens(content string) []string {
	terms := tokenizeContextTerms(content)
	if len(terms) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(terms))
	for _, term := range terms {
		if _, isStopword := createAutoLinkStopwords[term]; isStopword {
			continue
		}
		filtered = append(filtered, term)
	}
	return filtered
}

func normalizedTagSet(tags []string) map[string]struct{} {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func jaccardScore(a []string, b []string) float64 {
	return jaccardScoreFromSets(toSet(a), toSet(b))
}

func jaccardScoreFromSets(a map[string]struct{}, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for key := range a {
		if _, ok := b[key]; ok {
			intersection++
		}
	}
	if intersection == 0 {
		return 0
	}

	union := len(a) + len(b) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func toSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	return set
}
