package sync

import (
	"testing"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

func TestBuildSyncFocusProfile_LearnsFromNotesAndTasks(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_SYNC_FOCUS_TAGS_MAX", "5")
	t.Setenv("SB_SYNC_FOCUS_TERMS_MAX", "8")
	t.Setenv("SB_SYNC_FOCUS_NOTES_LIMIT", "50")
	t.Setenv("SB_SYNC_FOCUS_TASKS_LIMIT", "30")

	if _, err := s.CreateNote(
		"Orion billing migration shipped by @alice in #proj-orion",
		nil,
		[]string{"orion", "billing"},
		"manual",
	); err != nil {
		t.Fatalf("create note 1: %v", err)
	}
	if _, err := s.CreateNote(
		"Follow-up needed for Orion rollback plan",
		nil,
		[]string{"orion"},
		"manual",
	); err != nil {
		t.Fatalf("create note 2: %v", err)
	}
	if _, err := s.CreateNote(
		"Company all-hands schedule announced",
		nil,
		[]string{"announcement"},
		"sync",
	); err != nil {
		t.Fatalf("create note 3: %v", err)
	}

	task, err := s.CreateTask(
		"Finalize Orion billing rollout",
		"Coordinate with @alice and #proj-orion",
		nil,
		3,
	)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := s.UpdateTaskStatus(task.ID, model.TaskInProgress); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	svc := NewService(s, &mockExecutor{}, "")
	profile := svc.buildSyncFocusProfile(config.LoadRuntime())
	if profile == nil {
		t.Fatal("expected non-nil focus profile")
	}

	if len(profile.Tags) == 0 || profile.Tags[0] != "orion" {
		t.Fatalf("expected top tag 'orion', got %v", profile.Tags)
	}
	if !containsToken(profile.Terms, "orion") {
		t.Fatalf("expected profile terms to include 'orion', got %v", profile.Terms)
	}
	if !containsToken(profile.TaskTerms, "orion") {
		t.Fatalf("expected task terms to include 'orion', got %v", profile.TaskTerms)
	}
	if !containsToken(profile.TaskTerms, "@alice") {
		t.Fatalf("expected task terms to include '@alice', got %v", profile.TaskTerms)
	}
}

func TestBuildSyncFocusProfile_Disabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_SYNC_FOCUS_LEARNING", "0")

	if _, err := s.CreateNote("orion roadmap", nil, []string{"orion"}, "manual"); err != nil {
		t.Fatalf("create note: %v", err)
	}

	svc := NewService(s, &mockExecutor{}, "")
	profile := svc.buildSyncFocusProfile(config.LoadRuntime())
	if profile != nil {
		t.Fatalf("expected nil profile when feature disabled, got %+v", profile)
	}
}

func TestBuildSyncPrompt_WithAndWithoutProfile(t *testing.T) {
	withProfile := buildSyncPrompt(&focusProfile{
		Tags:      []string{"orion", "billing"},
		Terms:     []string{"migration", "rollout"},
		TaskTerms: []string{"@alice", "#proj-orion"},
	})
	if !contains(withProfile, "Priority tags: orion, billing") {
		t.Fatalf("prompt should include tags line, got:\n%s", withProfile)
	}
	if !contains(withProfile, "Active task keywords: @alice, #proj-orion") {
		t.Fatalf("prompt should include task keywords line, got:\n%s", withProfile)
	}

	withoutProfile := buildSyncPrompt(nil)
	if !contains(withoutProfile, "No stable profile yet.") {
		t.Fatalf("prompt should include fallback profile guidance, got:\n%s", withoutProfile)
	}
}

func containsToken(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
