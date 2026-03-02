package consolidation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

// mockAgent implements adapter.Agent for testing.
type mockAgent struct {
	result *adapter.ConsolidationResult
	err    error
}

func (m *mockAgent) Name() string { return "mock" }

func (m *mockAgent) Consolidate(ctx context.Context, req adapter.ConsolidationRequest) (*adapter.ConsolidationResult, error) {
	return m.result, m.err
}

func (m *mockAgent) Summarize(ctx context.Context, text string) (string, error) {
	return "summary", nil
}

func setupTest(t *testing.T) (*store.Store, *kb.KB) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	kbDir := filepath.Join(dir, "knowledge")
	k := kb.New(kbDir)
	return s, k
}

func TestProposeAndApply(t *testing.T) {
	s, k := setupTest(t)

	// Create and complete a session with data
	sess, _ := s.CreateSession("Test Session", "test goal")
	s.CreateTask("Task 1", "do something", &sess.ID, 1)
	s.CreateNote("interesting finding", &sess.ID, []string{"research"}, "manual")
	s.EndSession(sess.ID, "completed work")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Session focused on testing.",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "testing/approach.md",
					Content: "# Testing Approach\n\nUse table-driven tests.\n",
					Reason:  "Document testing approach",
				},
			},
			SuggestedTasks: []string{"Write more tests"},
		},
	}

	svc := NewService(s, k, agent)

	// Propose
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	if proposed.Summary != "Session focused on testing." {
		t.Errorf("unexpected summary: %q", proposed.Summary)
	}
	if len(proposed.KBUpdates) != 1 {
		t.Fatalf("expected 1 KB update, got %d", len(proposed.KBUpdates))
	}
	if !proposed.KBUpdates[0].IsNew {
		t.Error("expected IsNew to be true")
	}
	if len(proposed.SuggestedTasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(proposed.SuggestedTasks))
	}

	// Apply all
	err = svc.Apply(context.Background(), proposed, []int{0}, []int{0})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Verify KB file was written
	content, err := k.Read("testing/approach.md")
	if err != nil {
		t.Fatalf("read KB: %v", err)
	}
	if content != "# Testing Approach\n\nUse table-driven tests.\n" {
		t.Errorf("unexpected KB content: %q", content)
	}

	// Verify task was created
	tasks, _ := s.ListTasks(store.TaskFilter{})
	found := false
	for _, task := range tasks {
		if task.Title == "Write more tests" {
			found = true
			break
		}
	}
	if !found {
		t.Error("suggested task was not created")
	}

	// Verify consolidation was recorded
	consolidated, _ := s.HasBeenConsolidated(sess.ID)
	if !consolidated {
		t.Error("session should be marked as consolidated")
	}
}

func TestProposeActiveSession(t *testing.T) {
	s, k := setupTest(t)

	sess, _ := s.CreateSession("Active", "still working")
	agent := &mockAgent{}
	svc := NewService(s, k, agent)

	_, err := svc.Propose(context.Background(), sess.ID)
	if err == nil {
		t.Fatal("expected error for active session")
	}
}

func TestProposePartialApply(t *testing.T) {
	s, k := setupTest(t)

	sess, _ := s.CreateSession("Partial", "")
	s.EndSession(sess.ID, "")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Test",
			KBUpdates: []adapter.KBUpdate{
				{Path: "a.md", Content: "A", Reason: "a"},
				{Path: "b.md", Content: "B", Reason: "b"},
			},
			SuggestedTasks: []string{"task1", "task2"},
		},
	}

	svc := NewService(s, k, agent)
	proposed, _ := svc.Propose(context.Background(), sess.ID)

	// Only approve first KB file and second task
	svc.Apply(context.Background(), proposed, []int{0}, []int{1})

	// a.md should exist, b.md should not
	if !k.Exists("a.md") {
		t.Error("a.md should exist")
	}
	if k.Exists("b.md") {
		t.Error("b.md should not exist")
	}
}

func TestProposeExistingFile(t *testing.T) {
	s, k := setupTest(t)

	// Pre-create a KB file
	k.Write("existing.md", "# Old Content\n")

	sess, _ := s.CreateSession("Update", "")
	s.EndSession(sess.ID, "")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Updated existing file",
			KBUpdates: []adapter.KBUpdate{
				{Path: "existing.md", Content: "# New Content\n", Reason: "update"},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, _ := svc.Propose(context.Background(), sess.ID)

	if proposed.KBUpdates[0].IsNew {
		t.Error("expected IsNew to be false for existing file")
	}
	if proposed.KBUpdates[0].OldContent != "# Old Content\n" {
		t.Errorf("unexpected old content: %q", proposed.KBUpdates[0].OldContent)
	}
}
