package consolidation

import (
	"fmt"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

type sleepPolicyDecision struct {
	NoteID int64
	Score  float64
	Reason string
}

type sleepPolicyResult struct {
	CandidateCount int
	Threshold      float64
	SelectedNotes  []model.Note
	Decisions      []sleepPolicyDecision
}

func applySleepLongTermPolicy(notes []model.Note, now time.Time, cfg config.Runtime) sleepPolicyResult {
	result := sleepPolicyResult{
		CandidateCount: len(notes),
		Threshold:      cfg.SleepPolicyScoreThreshold,
	}
	if len(notes) == 0 {
		return result
	}

	now = now.UTC()
	for _, note := range notes {
		score, reason := scoreSleepLongTermCandidate(note, now, cfg)
		if score < cfg.SleepPolicyScoreThreshold {
			continue
		}
		result.SelectedNotes = append(result.SelectedNotes, note)
		result.Decisions = append(result.Decisions, sleepPolicyDecision{
			NoteID: note.ID,
			Score:  score,
			Reason: reason,
		})
	}
	return result
}

func scoreSleepLongTermCandidate(note model.Note, now time.Time, cfg config.Runtime) (float64, string) {
	recurrence := clamp01(float64(note.RecallCount) / 3.0)

	utility := clamp01((note.Salience * 0.60) + (note.Strength * 0.40))
	switch note.Source {
	case "manual":
		utility = clamp01(utility + 0.10)
	case "sync":
		utility = clamp01(utility + 0.03)
	}

	baseTime := note.UpdatedAt
	if note.LastRecalledAt != nil && note.LastRecalledAt.After(baseTime) {
		baseTime = *note.LastRecalledAt
	}
	ageDays := now.Sub(baseTime.UTC()).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	staleness := clamp01(ageDays / 30.0)

	score := (cfg.SleepPolicyRecurrenceW * recurrence) + (cfg.SleepPolicyUtilityW * utility) - (cfg.SleepPolicyStalenessW * staleness)
	score = clamp01(score)

	reason := fmt.Sprintf("score=%.2f recurrence=%.2f utility=%.2f staleness=%.2f", score, recurrence, utility, staleness)
	return score, reason
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
