package consolidation

import (
	"strings"
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

func TestApplySleepLongTermPolicy_SelectsHighSignalNote(t *testing.T) {
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	cfg := config.Runtime{
		SleepPolicyScoreThreshold: 0.20,
		SleepPolicyRecurrenceW:    0.35,
		SleepPolicyUtilityW:       0.55,
		SleepPolicyStalenessW:     0.25,
	}

	notes := []model.Note{
		{
			ID:          1,
			Source:      "manual",
			Salience:    0.75,
			Strength:    0.70,
			RecallCount: 2,
			UpdatedAt:   now.Add(-6 * time.Hour),
		},
		{
			ID:          2,
			Source:      "sync",
			Salience:    0.25,
			Strength:    0.20,
			RecallCount: 0,
			UpdatedAt:   now.Add(-2 * time.Hour),
		},
	}

	result := applySleepLongTermPolicy(notes, now, cfg)
	if result.CandidateCount != 2 {
		t.Fatalf("expected 2 candidates, got %d", result.CandidateCount)
	}
	if result.Threshold != 0.20 {
		t.Fatalf("unexpected threshold: %f", result.Threshold)
	}
	if len(result.SelectedNotes) != 1 {
		t.Fatalf("expected 1 selected note, got %d", len(result.SelectedNotes))
	}
	if result.SelectedNotes[0].ID != 1 {
		t.Fatalf("expected note #1 selected, got #%d", result.SelectedNotes[0].ID)
	}
	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if !strings.Contains(result.Decisions[0].Reason, "score=") {
		t.Fatalf("expected reason to include score, got %q", result.Decisions[0].Reason)
	}
}

func TestScoreSleepLongTermCandidate_StalenessPenalty(t *testing.T) {
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	cfg := config.Runtime{
		SleepPolicyRecurrenceW: 0.35,
		SleepPolicyUtilityW:    0.55,
		SleepPolicyStalenessW:  0.25,
	}

	fresh := model.Note{
		ID:        1,
		Source:    "manual",
		Salience:  0.50,
		Strength:  0.50,
		UpdatedAt: now.Add(-1 * time.Hour),
	}
	stale := fresh
	stale.ID = 2
	stale.UpdatedAt = now.Add(-90 * 24 * time.Hour)

	freshScore, _ := scoreSleepLongTermCandidate(fresh, now, cfg)
	staleScore, _ := scoreSleepLongTermCandidate(stale, now, cfg)
	if staleScore >= freshScore {
		t.Fatalf("stale score should be lower: stale=%f fresh=%f", staleScore, freshScore)
	}
}
