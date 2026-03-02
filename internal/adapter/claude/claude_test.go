package claude

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/model"
)

// mockExecutor returns pre-canned responses for testing.
type mockExecutor struct {
	output []byte
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	return m.output, m.err
}

func TestConsolidate(t *testing.T) {
	// Build a mock response matching claude CLI JSON envelope
	inner := consolidationOutput{
		Summary: "Implemented adapter pattern for the API layer.",
		KBUpdates: []kbUpdate{
			{
				Path:    "architecture/adapter-pattern.md",
				Content: "# Adapter Pattern\n\nUse interfaces to decouple.\n",
				Reason:  "Document the adapter pattern",
			},
		},
		SuggestedTasks: []string{"Add tests for adapter interfaces"},
	}
	innerJSON, _ := json.Marshal(inner)

	envelope := claudeJSONResponse{
		Type:    "result",
		Result:  string(innerJSON),
		IsError: false,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	agent := New(WithExecutor(&mockExecutor{output: envelopeJSON}))

	now := time.Now()
	req := adapter.ConsolidationRequest{
		Session: &model.Session{
			ID:        1,
			Title:     "Test Session",
			Goal:      "test",
			Status:    model.SessionCompleted,
			StartedAt: now,
		},
		Events: []model.Event{
			{Type: model.EventSessionStarted, Payload: `{"title":"Test Session"}`, CreatedAt: now},
		},
		Notes: []model.Note{
			{Content: "adapter pattern is useful", Tags: []string{"architecture"}},
		},
		Tasks: []model.Task{
			{Title: "Implement adapter", Status: model.TaskDone},
		},
		ExistingKB: []string{"golang-tips.md"},
	}

	result, err := agent.Consolidate(context.Background(), req)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}

	if result.Summary != "Implemented adapter pattern for the API layer." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
	if len(result.KBUpdates) != 1 {
		t.Fatalf("expected 1 KB update, got %d", len(result.KBUpdates))
	}
	if result.KBUpdates[0].Path != "architecture/adapter-pattern.md" {
		t.Errorf("unexpected path: %q", result.KBUpdates[0].Path)
	}
	if len(result.SuggestedTasks) != 1 {
		t.Fatalf("expected 1 suggested task, got %d", len(result.SuggestedTasks))
	}
}

func TestConsolidateDirectJSON(t *testing.T) {
	// Test fallback: output is direct JSON (no envelope)
	inner := consolidationOutput{
		Summary:        "Direct JSON test.",
		KBUpdates:      []kbUpdate{},
		SuggestedTasks: []string{},
	}
	innerJSON, _ := json.Marshal(inner)

	agent := New(WithExecutor(&mockExecutor{output: innerJSON}))

	now := time.Now()
	req := adapter.ConsolidationRequest{
		Session: &model.Session{
			ID:        1,
			Title:     "Test",
			StartedAt: now,
		},
	}

	result, err := agent.Consolidate(context.Background(), req)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if result.Summary != "Direct JSON test." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
}

func TestConsolidateJSONBlock(t *testing.T) {
	// Test fallback: JSON embedded in markdown
	inner := consolidationOutput{
		Summary:        "Markdown block test.",
		KBUpdates:      []kbUpdate{},
		SuggestedTasks: []string{"Follow up"},
	}
	innerJSON, _ := json.Marshal(inner)
	markdown := "Here is the result:\n\n```json\n" + string(innerJSON) + "\n```\n"

	agent := New(WithExecutor(&mockExecutor{output: []byte(markdown)}))

	now := time.Now()
	req := adapter.ConsolidationRequest{
		Session: &model.Session{ID: 1, Title: "Test", StartedAt: now},
	}

	result, err := agent.Consolidate(context.Background(), req)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if result.Summary != "Markdown block test." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
}

func TestBuildPrompt(t *testing.T) {
	now := time.Now()
	session := &model.Session{
		Title:     "Test Session",
		Goal:      "testing",
		Status:    model.SessionCompleted,
		StartedAt: now,
	}
	events := []model.Event{
		{Type: model.EventSessionStarted, Payload: `{"title":"Test"}`, CreatedAt: now},
	}
	notes := []model.Note{
		{Content: "a note", Tags: []string{"tag1"}},
	}
	tasks := []model.Task{
		{Title: "a task", Status: model.TaskDone},
	}
	kbFiles := []string{"existing.md"}

	prompt := buildConsolidationPrompt(session, events, notes, tasks, kbFiles)

	if prompt == "" {
		t.Fatal("empty prompt")
	}
	// Check that key elements are present
	for _, want := range []string{"Test Session", "testing", "a note", "tag1", "a task", "done", "existing.md"} {
		if !contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestName(t *testing.T) {
	agent := New()
	if agent.Name() != "claude-code" {
		t.Errorf("unexpected name: %q", agent.Name())
	}
}
