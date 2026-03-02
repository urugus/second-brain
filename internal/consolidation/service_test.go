package consolidation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

// mockAgent implements adapter.Agent for testing.
type mockAgent struct {
	result  *adapter.ConsolidationResult
	err     error
	lastReq adapter.ConsolidationRequest
}

func (m *mockAgent) Name() string { return "mock" }

func (m *mockAgent) Consolidate(ctx context.Context, req adapter.ConsolidationRequest) (*adapter.ConsolidationResult, error) {
	m.lastReq = req
	return m.result, m.err
}

func (m *mockAgent) Summarize(ctx context.Context, text string) (string, error) {
	return "summary", nil
}

func setupTest(t *testing.T) (*store.Store, *kb.KB) {
	t.Helper()
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "1")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "1")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "0.18")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "0.35")

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

func TestSleepConsolidate_ReplayDedupAndStrengthUpdate(t *testing.T) {
	s, k := setupTest(t)

	n1, _ := s.CreateNote("Go interfaces for abstraction", nil, []string{"go"}, "manual")
	n2, _ := s.CreateNote("  go interfaces for abstraction ", nil, []string{"go", "design"}, "sync")
	n3, _ := s.CreateNote("Use table-driven tests for parsers", nil, []string{"testing"}, "manual")

	beforeStrength := map[int64]float64{
		n1.ID: n1.Strength,
		n2.ID: n2.Strength,
		n3.ID: n3.Strength,
	}

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Sleep consolidation completed.",
			KBUpdates: []adapter.KBUpdate{
				{Path: "golang/interfaces.md", Content: "# Interfaces v1\n", Reason: "initial"},
				{Path: "golang/interfaces.md", Content: "# Interfaces v2\n", Reason: "deduped latest"},
				{Path: "testing/table-driven.md", Content: "# Table Driven\n", Reason: "testing"},
			},
			SuggestedTasks: []string{
				"Review interface boundaries",
				"Review interface boundaries",
			},
		},
	}

	svc := NewService(s, k, agent)
	result, err := svc.SleepConsolidate(context.Background(), 1)
	if err != nil {
		t.Fatalf("sleep consolidate: %v", err)
	}
	if result == nil {
		t.Fatal("expected sleep result")
	}

	if result.NotesProcessed != 3 {
		t.Fatalf("expected 3 processed notes, got %d", result.NotesProcessed)
	}
	if result.NotesReplayed != 2 {
		t.Fatalf("expected 2 replayed notes after dedupe, got %d", result.NotesReplayed)
	}
	if result.DuplicatesMerged != 1 {
		t.Fatalf("expected 1 merged duplicate, got %d", result.DuplicatesMerged)
	}

	if agent.lastReq.Mode != "sleep" {
		t.Fatalf("expected sleep mode request, got %q", agent.lastReq.Mode)
	}
	if len(agent.lastReq.Notes) != 2 {
		t.Fatalf("expected deduped replay notes length 2, got %d", len(agent.lastReq.Notes))
	}
	seen := map[string]struct{}{}
	for _, note := range agent.lastReq.Notes {
		key := normalizeNoteContent(note.Content)
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate replay note key found: %q", key)
		}
		seen[key] = struct{}{}
	}

	content, err := k.Read("golang/interfaces.md")
	if err != nil {
		t.Fatalf("read deduped KB file: %v", err)
	}
	if content != "# Interfaces v2\n" {
		t.Fatalf("expected latest duplicate KB update to win, got %q", content)
	}
	if len(result.KBFilesUpdated) != 2 {
		t.Fatalf("expected 2 unique KB files updated, got %d", len(result.KBFilesUpdated))
	}

	tasks, _ := s.ListTasks(store.TaskFilter{})
	count := 0
	for _, task := range tasks {
		if task.Title == "Review interface boundaries" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected deduped suggested task count 1, got %d", count)
	}

	afterNotes, err := s.ListNotes(store.NoteFilter{})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	for _, note := range afterNotes {
		before, ok := beforeStrength[note.ID]
		if !ok {
			continue
		}
		if note.ConsolidatedAt == nil {
			t.Fatalf("note %d should be consolidated", note.ID)
		}
		if note.Strength <= before {
			t.Fatalf("note %d strength should increase (before=%.4f after=%.4f)", note.ID, before, note.Strength)
		}
	}
}

func TestSleepConsolidate_AllKBWritesFail(t *testing.T) {
	s, k := setupTest(t)

	note, _ := s.CreateNote("important note", nil, []string{"sync"}, "sync")
	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Should fail write",
			KBUpdates: []adapter.KBUpdate{
				{Path: "../outside.md", Content: "x", Reason: "invalid path"},
			},
		},
	}
	svc := NewService(s, k, agent)

	result, err := svc.SleepConsolidate(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when all KB writes fail")
	}
	if result != nil {
		t.Fatal("expected nil result on total KB write failure")
	}
	if !strings.Contains(err.Error(), "all KB writes failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if after.ConsolidatedAt != nil {
		t.Fatalf("note %d should remain unconsolidated on failure", note.ID)
	}
}

func TestSleepConsolidate_RecordsPredictionError(t *testing.T) {
	s, k := setupTest(t)

	if _, err := s.CreateNote("predictive sleep note", nil, []string{"ops"}, "sync"); err != nil {
		t.Fatalf("create note: %v", err)
	}

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "sleep summary",
			KBUpdates: []adapter.KBUpdate{
				{Path: "ops/sleep.md", Content: "# Sleep\n", Reason: "record"},
			},
		},
	}

	svc := NewService(s, k, agent)
	if _, err := svc.SleepConsolidate(context.Background(), 1); err != nil {
		t.Fatalf("sleep consolidate: %v", err)
	}

	logs, err := s.ListPredictionErrors(5)
	if err != nil {
		t.Fatalf("list prediction logs: %v", err)
	}
	found := false
	for _, log := range logs {
		if log.Source != "sleep" {
			continue
		}
		if log.Metric == "kb_updates" {
			found = true
			if log.ActualValue != 1 {
				t.Fatalf("expected actual kb updates=1, got %f", log.ActualValue)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected sleep kb_updates prediction log")
	}
}

func TestSleepConsolidate_SleepReplayDisabled(t *testing.T) {
	s, k := setupTest(t)
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "0")

	n1, _ := s.CreateNote("same note", nil, nil, "manual")
	n2, _ := s.CreateNote("same note", nil, nil, "sync")
	before := map[int64]float64{
		n1.ID: n1.Strength,
		n2.ID: n2.Strength,
	}

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "legacy sleep",
			KBUpdates: []adapter.KBUpdate{
				{Path: "legacy/sleep.md", Content: "# Legacy\n", Reason: "legacy"},
			},
		},
	}
	svc := NewService(s, k, agent)
	result, err := svc.SleepConsolidate(context.Background(), 1)
	if err != nil {
		t.Fatalf("sleep consolidate: %v", err)
	}
	if result == nil {
		t.Fatal("expected sleep result")
	}
	if result.DuplicatesMerged != 0 {
		t.Fatalf("expected no dedupe when replay disabled, got %d", result.DuplicatesMerged)
	}
	if len(agent.lastReq.Notes) != 2 {
		t.Fatalf("expected all notes passed through when replay disabled, got %d", len(agent.lastReq.Notes))
	}

	after1, _ := s.GetNote(n1.ID)
	after2, _ := s.GetNote(n2.ID)
	if after1.ConsolidatedAt == nil || after2.ConsolidatedAt == nil {
		t.Fatal("expected notes to be consolidated")
	}
	if !almostEqual(before[n1.ID], after1.Strength) {
		t.Fatalf("strength should remain unchanged when replay disabled: before=%f after=%f", before[n1.ID], after1.Strength)
	}
	if !almostEqual(before[n2.ID], after2.Strength) {
		t.Fatalf("strength should remain unchanged when replay disabled: before=%f after=%f", before[n2.ID], after2.Strength)
	}
}

func almostEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}
