package consolidation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/model"
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
	t.Setenv("SB_SLEEP_POLICY_SCORE_THRESHOLD", "0.20")
	t.Setenv("SB_SLEEP_POLICY_RECURRENCE_WEIGHT", "0.35")
	t.Setenv("SB_SLEEP_POLICY_UTILITY_WEIGHT", "0.55")
	t.Setenv("SB_SLEEP_POLICY_STALENESS_WEIGHT", "0.25")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_AUTOLINK", "1")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_WEIGHT", "0.12")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_MAX_PAIRS", "24")

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
	if !strings.Contains(err.Error(), "kb writes failed") {
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

func TestSleepConsolidate_PartialKBWritesFailAndKeepNotesUnconsolidated(t *testing.T) {
	s, k := setupTest(t)

	note, _ := s.CreateNote("partial failure note", nil, []string{"ops"}, "manual")
	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Should partially fail write",
			KBUpdates: []adapter.KBUpdate{
				{Path: "ops/ok.md", Content: "# OK\n", Reason: "valid path"},
				{Path: "../outside.md", Content: "x", Reason: "invalid path"},
			},
			SuggestedTasks: []string{"task should not be created"},
		},
	}
	svc := NewService(s, k, agent)

	result, err := svc.SleepConsolidate(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when some KB writes fail")
	}
	if result != nil {
		t.Fatal("expected nil result on partial KB write failure")
	}
	if !strings.Contains(err.Error(), "kb writes failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Some files may already be written before the failure is detected.
	if !k.Exists("ops/ok.md") {
		t.Fatal("expected valid KB write to be present")
	}

	after, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if after.ConsolidatedAt != nil {
		t.Fatalf("note %d should remain unconsolidated on partial failure", note.ID)
	}

	tasks, err := s.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	for _, task := range tasks {
		if task.Title == "task should not be created" {
			t.Fatal("suggested tasks must not be created when KB writes fail")
		}
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

func TestSleepConsolidate_FiltersByPolicyScore(t *testing.T) {
	s, k := setupTest(t)
	t.Setenv("SB_SLEEP_POLICY_SCORE_THRESHOLD", "0.28")

	highSignal, _ := s.CreateNote("high signal", nil, nil, "manual")
	lowSignal, _ := s.CreateNote("low signal", nil, nil, "sync")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "policy filter test",
			KBUpdates: []adapter.KBUpdate{
				{Path: "policy/filter.md", Content: "# Policy\n", Reason: "test"},
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
	if result.PolicyCandidates != 2 {
		t.Fatalf("expected 2 policy candidates, got %d", result.PolicyCandidates)
	}
	if result.PolicySelected != 1 {
		t.Fatalf("expected 1 policy-selected note, got %d", result.PolicySelected)
	}
	if len(agent.lastReq.Notes) != 1 {
		t.Fatalf("expected 1 note passed to sleep agent, got %d", len(agent.lastReq.Notes))
	}
	if agent.lastReq.Notes[0].ID != highSignal.ID {
		t.Fatalf("expected high-signal note #%d to be selected, got #%d", highSignal.ID, agent.lastReq.Notes[0].ID)
	}

	afterHigh, _ := s.GetNote(highSignal.ID)
	afterLow, _ := s.GetNote(lowSignal.ID)
	if afterHigh.ConsolidatedAt == nil {
		t.Fatalf("high-signal note %d should be consolidated", highSignal.ID)
	}
	if afterLow.ConsolidatedAt != nil {
		t.Fatalf("low-signal note %d should remain unconsolidated", lowSignal.ID)
	}
}

func TestSleepConsolidate_SkipsWhenPolicySelectedBelowThreshold(t *testing.T) {
	s, k := setupTest(t)
	t.Setenv("SB_SLEEP_POLICY_SCORE_THRESHOLD", "0.28")

	_, _ = s.CreateNote("high signal", nil, nil, "manual")
	_, _ = s.CreateNote("low signal", nil, nil, "sync")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "should not run",
			KBUpdates: []adapter.KBUpdate{
				{Path: "policy/skip.md", Content: "# Skip\n", Reason: "test"},
			},
		},
	}

	svc := NewService(s, k, agent)
	result, err := svc.SleepConsolidate(context.Background(), 2)
	if err != nil {
		t.Fatalf("sleep consolidate: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when selected notes are below threshold, got %+v", result)
	}
	if agent.lastReq.Mode != "" {
		t.Fatal("agent should not be called when selected notes are below threshold")
	}
	if k.Exists("policy/skip.md") {
		t.Fatal("KB should not be updated when sleep consolidation is skipped")
	}
}

func TestSleepConsolidate_AutoLinksCooccurredNotes(t *testing.T) {
	s, k := setupTest(t)

	n1, _ := s.CreateNote("session boundary strategy", nil, []string{"session"}, "manual")
	n2, _ := s.CreateNote("state transition policy", nil, []string{"state"}, "manual")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "sleep autolink test",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "sleep/autolink.md",
					Content: "# Sleep AutoLink\nsession boundary strategy\nstate transition policy\n",
					Reason:  "co-occurred memories",
				},
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

	fromFirst, err := s.RelatedNotes(n1.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n1: %v", err)
	}
	if !containsRelatedNote(fromFirst, n2.ID) {
		t.Fatalf("expected note %d to auto-link to %d, got %+v", n1.ID, n2.ID, fromFirst)
	}

	fromSecond, err := s.RelatedNotes(n2.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n2: %v", err)
	}
	if !containsRelatedNote(fromSecond, n1.ID) {
		t.Fatalf("expected note %d to auto-link to %d, got %+v", n2.ID, n1.ID, fromSecond)
	}
}

func TestApplyAppendsRelatedSection(t *testing.T) {
	s, k := setupTest(t)

	// Pre-create a related KB file with a mapped note
	k.Write("existing/related.md", "# Existing Related\nSome content.\n")
	existingNote, _ := s.CreateNote("existing knowledge", nil, nil, "manual")
	s.MapKBNotes("existing/related.md", []int64{existingNote.ID})

	// Create session with a note linked to the existing note
	sess, _ := s.CreateSession("Link Test", "test related links")
	sessionNote, _ := s.CreateNote("new knowledge", &sess.ID, nil, "manual")
	s.EndSession(sess.ID, "done")

	// Create a memory edge between the session note and the existing note
	s.LinkNotes(sessionNote.ID, existingNote.ID, 0.8, "conceptual link")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Linked consolidation",
			KBUpdates: []adapter.KBUpdate{
				{Path: "new/topic.md", Content: "# New Topic\nNew content.\n", Reason: "new topic"},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	err = svc.Apply(context.Background(), proposed, []int{0}, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	content, err := k.Read("new/topic.md")
	if err != nil {
		t.Fatalf("read KB: %v", err)
	}

	// Should contain the Related section with a link to existing/related.md
	if !strings.Contains(content, "## Related") {
		t.Fatalf("expected Related section in KB file, got:\n%s", content)
	}
	if !strings.Contains(content, "Existing Related") {
		t.Fatalf("expected link title 'Existing Related' in Related section, got:\n%s", content)
	}
	if !strings.Contains(content, "existing/related.md") || !strings.Contains(content, "../") {
		t.Fatalf("expected relative path to existing/related.md in Related section, got:\n%s", content)
	}
}

func TestApplyRelatedSectionUsesContentMatchedNotes(t *testing.T) {
	s, k := setupTest(t)

	k.Write("existing/alpha.md", "# Related Alpha\n")
	k.Write("existing/beta.md", "# Related Beta\n")
	anchorAlpha, _ := s.CreateNote("anchor alpha", nil, nil, "manual")
	anchorBeta, _ := s.CreateNote("anchor beta", nil, nil, "manual")
	s.MapKBNotes("existing/alpha.md", []int64{anchorAlpha.ID})
	s.MapKBNotes("existing/beta.md", []int64{anchorBeta.ID})

	sess, _ := s.CreateSession("Selective Mapping", "")
	noteA, _ := s.CreateNote("alpha cache invalidation details", &sess.ID, []string{"alpha"}, "manual")
	noteB, _ := s.CreateNote("beta queue backpressure analysis", &sess.ID, []string{"beta"}, "manual")
	s.EndSession(sess.ID, "done")

	s.LinkNotes(noteA.ID, anchorAlpha.ID, 0.8, "alpha relation")
	s.LinkNotes(noteB.ID, anchorBeta.ID, 0.8, "beta relation")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Selective related mapping",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "new/alpha-topic.md",
					Content: "# Alpha Topic\nalpha cache invalidation details\n",
					Reason:  "alpha",
				},
				{
					Path:    "new/beta-topic.md",
					Content: "# Beta Topic\nbeta queue backpressure analysis\n",
					Reason:  "beta",
				},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if err := svc.Apply(context.Background(), proposed, []int{0, 1}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	alphaContent, err := k.Read("new/alpha-topic.md")
	if err != nil {
		t.Fatalf("read alpha topic: %v", err)
	}
	betaContent, err := k.Read("new/beta-topic.md")
	if err != nil {
		t.Fatalf("read beta topic: %v", err)
	}

	if !strings.Contains(alphaContent, "Related Alpha") {
		t.Fatalf("alpha topic should include Related Alpha link, got:\n%s", alphaContent)
	}
	if strings.Contains(alphaContent, "Related Beta") {
		t.Fatalf("alpha topic should not include Related Beta link, got:\n%s", alphaContent)
	}
	if !strings.Contains(betaContent, "Related Beta") {
		t.Fatalf("beta topic should include Related Beta link, got:\n%s", betaContent)
	}
	if strings.Contains(betaContent, "Related Alpha") {
		t.Fatalf("beta topic should not include Related Alpha link, got:\n%s", betaContent)
	}
}

func TestApplyAutoLinksCooccurredNotes(t *testing.T) {
	s, k := setupTest(t)

	sess, _ := s.CreateSession("AutoLink Apply", "")
	n1, _ := s.CreateNote("cache invalidation edge case", &sess.ID, []string{"cache"}, "manual")
	n2, _ := s.CreateNote("ttl refresh backoff policy", &sess.ID, []string{"ttl"}, "manual")
	s.EndSession(sess.ID, "done")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "apply autolink test",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "apply/autolink.md",
					Content: "# AutoLink\ncache invalidation edge case\nttl refresh backoff policy\n",
					Reason:  "co-occurred notes",
				},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if err := svc.Apply(context.Background(), proposed, []int{0}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	fromFirst, err := s.RelatedNotes(n1.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n1: %v", err)
	}
	if !containsRelatedNote(fromFirst, n2.ID) {
		t.Fatalf("expected note %d to auto-link to %d, got %+v", n1.ID, n2.ID, fromFirst)
	}

	fromSecond, err := s.RelatedNotes(n2.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n2: %v", err)
	}
	if !containsRelatedNote(fromSecond, n1.ID) {
		t.Fatalf("expected note %d to auto-link to %d, got %+v", n2.ID, n1.ID, fromSecond)
	}
}

func TestApplyAutoLinkRespectsGlobalPairCap(t *testing.T) {
	s, k := setupTest(t)
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_MAX_PAIRS", "1")

	sess, _ := s.CreateSession("AutoLink Cap", "")
	n1, _ := s.CreateNote("cap note one", &sess.ID, nil, "manual")
	n2, _ := s.CreateNote("cap note two", &sess.ID, nil, "manual")
	n3, _ := s.CreateNote("cap note three", &sess.ID, nil, "manual")
	s.EndSession(sess.ID, "done")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "apply autolink cap test",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "apply/cap-a.md",
					Content: "# Cap A\ncap note one\ncap note two\ncap note three\n",
					Reason:  "cap coverage a",
				},
				{
					Path:    "apply/cap-b.md",
					Content: "# Cap B\ncap note one\ncap note two\ncap note three\n",
					Reason:  "cap coverage b",
				},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if err := svc.Apply(context.Background(), proposed, []int{0, 1}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	fromFirst, err := s.RelatedNotes(n1.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n1: %v", err)
	}
	if !containsRelatedNote(fromFirst, n2.ID) {
		t.Fatalf("expected capped pair n1<->n2 to exist, got %+v", fromFirst)
	}
	if containsRelatedNote(fromFirst, n3.ID) {
		t.Fatalf("did not expect capped run to link n1->n3, got %+v", fromFirst)
	}

	fromThird, err := s.RelatedNotes(n3.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n3: %v", err)
	}
	if len(fromThird) != 0 {
		t.Fatalf("did not expect note %d to be linked when cap=1, got %+v", n3.ID, fromThird)
	}
}

func TestApplyAutoLinkDisabled(t *testing.T) {
	s, k := setupTest(t)
	t.Setenv("SB_FEATURE_MEMORY_EDGE_AUTOLINK", "0")

	sess, _ := s.CreateSession("AutoLink Disabled", "")
	n1, _ := s.CreateNote("idempotent deployment checklist", &sess.ID, []string{"deploy"}, "manual")
	n2, _ := s.CreateNote("rollback safety window", &sess.ID, []string{"deploy"}, "manual")
	s.EndSession(sess.ID, "done")

	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "apply autolink disabled test",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "apply/autolink-disabled.md",
					Content: "# AutoLink Disabled\nidempotent deployment checklist\nrollback safety window\n",
					Reason:  "co-occurred notes",
				},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, err := svc.Propose(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if err := svc.Apply(context.Background(), proposed, []int{0}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	fromFirst, err := s.RelatedNotes(n1.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes n1: %v", err)
	}
	if containsRelatedNote(fromFirst, n2.ID) {
		t.Fatalf("did not expect auto-link when feature disabled, got %+v", fromFirst)
	}
}

func TestApplyStripsOldRelatedSection(t *testing.T) {
	s, k := setupTest(t)

	sess, _ := s.CreateSession("Rewrite Test", "")
	s.CreateNote("note", &sess.ID, nil, "manual")
	s.EndSession(sess.ID, "done")

	// Agent returns content that already has a Related section
	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Strip test",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "strip.md",
					Content: "# Strip\nContent.\n\n---\n\n## Related\n- [Old](old.md)\n",
					Reason:  "test",
				},
			},
		},
	}

	svc := NewService(s, k, agent)
	proposed, _ := svc.Propose(context.Background(), sess.ID)
	svc.Apply(context.Background(), proposed, []int{0}, nil)

	content, _ := k.Read("strip.md")
	// Old Related section should be stripped
	if strings.Contains(content, "old.md") {
		t.Fatalf("old Related links should be stripped, got:\n%s", content)
	}
	// Content body should be preserved
	if !strings.Contains(content, "# Strip") || !strings.Contains(content, "Content.") {
		t.Fatalf("main content should be preserved, got:\n%s", content)
	}
}

func almostEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

func containsRelatedNote(related []model.RelatedNote, noteID int64) bool {
	for _, rn := range related {
		if rn.Note.ID == noteID {
			return true
		}
	}
	return false
}
