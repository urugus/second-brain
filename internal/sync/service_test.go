package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

type mockExecutor struct {
	output []byte
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	return m.output, m.err
}

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "1")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "5")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "5")
	t.Setenv("SB_TASK_PRIORITY_MAX", "5")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_DECAY", "1")
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "0.010")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "0.02")
	t.Setenv("SB_FEATURE_ENTITY_LEARNING", "1")
	t.Setenv("SB_FEATURE_ENTITY_DECAY", "1")
	t.Setenv("SB_ENTITY_DECAY_RATE", "0.008")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.10")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.20")

	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSyncRun_Success(t *testing.T) {
	s := setupTestStore(t)

	syncResult := SyncResult{
		Summary:        "Synced 2 items from Slack",
		NotesAdded:     1,
		TasksAdded:     1,
		KBFilesUpdated: []string{"projects/alpha.md"},
	}
	envelope := claudeJSONResponse{
		Type:             "result",
		StructuredOutput: &syncResult,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}

	if result.Summary != "Synced 2 items from Slack" {
		t.Errorf("expected summary 'Synced 2 items from Slack', got %q", result.Summary)
	}
	if result.NotesAdded != 1 {
		t.Errorf("expected 1 note, got %d", result.NotesAdded)
	}
	if result.TasksAdded != 1 {
		t.Errorf("expected 1 task, got %d", result.TasksAdded)
	}

	// Verify log was created and completed
	logs, err := s.ListSyncLogs(1)
	if err != nil {
		t.Fatalf("list sync logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 sync log, got %d", len(logs))
	}
	if logs[0].Status != model.SyncCompleted {
		t.Errorf("expected completed, got %s", logs[0].Status)
	}
	if logs[0].NotesAdded != 1 {
		t.Errorf("expected 1 note in log, got %d", logs[0].NotesAdded)
	}
}

func TestSyncRun_AppliesMemoryDecay(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("decay target from sync run", nil, nil, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	// Seed note timestamps in the past so sync-run decay has measurable effect.
	if err := s.RecallNote(note.ID, time.Now().UTC().Add(-72*time.Hour), "seed"); err != nil {
		t.Fatalf("seed historical recall: %v", err)
	}
	beforeDecay, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note before sync run: %v", err)
	}

	syncResult := SyncResult{
		Summary:        "Sync with decay",
		NotesAdded:     0,
		TasksAdded:     0,
		KBFilesUpdated: []string{},
	}
	envelope := claudeJSONResponse{
		Type:             "result",
		StructuredOutput: &syncResult,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.DecayedNotes < 1 {
		t.Fatalf("expected at least 1 decayed note, got %d", result.DecayedNotes)
	}

	afterDecay, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after sync run: %v", err)
	}
	if afterDecay.Strength >= beforeDecay.Strength {
		t.Fatalf(
			"expected note strength to decay during sync run: before=%f after=%f",
			beforeDecay.Strength,
			afterDecay.Strength,
		)
	}
}

func TestSyncRun_AppliesMemoryEdgeDecay(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "1.0")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "0.01")

	a, _ := s.CreateNote("edge decay source", nil, nil, "manual")
	b, _ := s.CreateNote("edge decay target", nil, nil, "manual")
	if err := s.LinkNotes(a.ID, b.ID, 0.90, "sync-edge-decay"); err != nil {
		t.Fatalf("link notes: %v", err)
	}

	beforeRelated, err := s.RelatedNotes(a.ID, 1, 5)
	if err != nil {
		t.Fatalf("related notes before sync: %v", err)
	}
	if len(beforeRelated) == 0 {
		t.Fatal("expected related note before sync")
	}
	beforeScore := beforeRelated[0].Score

	// updated_at precision is seconds; ensure measurable dt.
	time.Sleep(1100 * time.Millisecond)

	syncResult := SyncResult{
		Summary:        "Sync with edge decay",
		NotesAdded:     0,
		TasksAdded:     0,
		KBFilesUpdated: []string{},
	}
	envelope := claudeJSONResponse{
		Type:             "result",
		StructuredOutput: &syncResult,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.DecayedEdges < 1 {
		t.Fatalf("expected at least 1 decayed edge, got %d", result.DecayedEdges)
	}

	afterRelated, err := s.RelatedNotes(a.ID, 1, 5)
	if err != nil {
		t.Fatalf("related notes after sync: %v", err)
	}
	if len(afterRelated) == 0 {
		t.Fatal("expected related note after sync")
	}
	afterScore := afterRelated[0].Score
	if afterScore >= beforeScore {
		t.Fatalf("expected edge score to decay during sync run: before=%f after=%f", beforeScore, afterScore)
	}
}

func TestSyncRun_AppliesEntityDecay(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_DECAY_RATE", "1.0")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.10")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.20")
	t.Setenv("SB_ENTITY_FEEDBACK_ALPHA", "0.25")

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}

	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before sync run: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one learned entity")
	}

	seedRecallAt := time.Now().UTC().Add(-96 * time.Hour)
	for i := 0; i < 3; i++ {
		if err := s.RecallNote(note.ID, seedRecallAt, "grace hopper compiler"); err != nil {
			t.Fatalf("seed recall %d: %v", i+1, err)
		}
	}
	seededEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after seed recalls: %v", err)
	}
	if len(seededEntities) == 0 {
		t.Fatal("expected learned entity after seed recalls")
	}
	beforeStrength := seededEntities[0].Strength

	syncResult := SyncResult{
		Summary:        "Sync with entity decay",
		NotesAdded:     0,
		TasksAdded:     0,
		KBFilesUpdated: []string{},
	}
	envelope := claudeJSONResponse{
		Type:             "result",
		StructuredOutput: &syncResult,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.DecayedEntities < 1 {
		t.Fatalf("expected at least 1 decayed entity, got %d", result.DecayedEntities)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after sync run: %v", err)
	}
	if len(afterEntities) == 0 {
		t.Fatal("expected learned entity after sync run")
	}
	if afterEntities[0].Strength >= beforeStrength {
		t.Fatalf("expected entity strength to decay during sync run: before=%f after=%f", beforeStrength, afterEntities[0].Strength)
	}
}

func TestSyncRun_ClaudeError(t *testing.T) {
	s := setupTestStore(t)

	svc := NewService(s, &mockExecutor{
		output: []byte("error output"),
		err:    fmt.Errorf("exit status 1"),
	}, "")

	_, err := svc.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify log shows failed
	logs, _ := s.ListSyncLogs(1)
	if len(logs) != 1 || logs[0].Status != model.SyncFailed {
		t.Error("expected failed sync log")
	}
}

func TestSyncRun_EmptySync(t *testing.T) {
	s := setupTestStore(t)

	syncResult := SyncResult{
		Summary:        "No important information found",
		NotesAdded:     0,
		TasksAdded:     0,
		KBFilesUpdated: []string{},
	}
	envelope := claudeJSONResponse{
		Type:             "result",
		StructuredOutput: &syncResult,
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.NotesAdded != 0 || result.TasksAdded != 0 {
		t.Errorf("expected empty sync, got notes=%d tasks=%d", result.NotesAdded, result.TasksAdded)
	}
}

func TestSyncRun_FallbackResultField(t *testing.T) {
	s := setupTestStore(t)

	syncResult := SyncResult{
		Summary:    "Synced via result field",
		NotesAdded: 2,
	}
	resultJSON, _ := json.Marshal(syncResult)
	envelope := claudeJSONResponse{
		Type:   "result",
		Result: string(resultJSON),
	}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.NotesAdded != 2 {
		t.Errorf("expected 2 notes, got %d", result.NotesAdded)
	}
}

func TestSyncRun_DirectJSON(t *testing.T) {
	s := setupTestStore(t)

	syncResult := SyncResult{
		Summary:    "Direct JSON",
		TasksAdded: 3,
	}
	directJSON, _ := json.Marshal(syncResult)

	svc := NewService(s, &mockExecutor{output: directJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.TasksAdded != 3 {
		t.Errorf("expected 3 tasks, got %d", result.TasksAdded)
	}
}

func TestCronSchedule(t *testing.T) {
	tests := []struct {
		interval time.Duration
		want     string
	}{
		{15 * time.Minute, "*/15 * * * *"},
		{30 * time.Minute, "*/30 * * * *"},
		{1 * time.Hour, "0 */1 * * *"},
		{6 * time.Hour, "0 */6 * * *"},
		{24 * time.Hour, "0 0 */1 * *"},
	}
	for _, tt := range tests {
		got := cronSchedule(tt.interval)
		if got != tt.want {
			t.Errorf("cronSchedule(%v) = %q, want %q", tt.interval, got, tt.want)
		}
	}
}

func TestParseSyncResponse_ClaudeError(t *testing.T) {
	envelope := claudeJSONResponse{
		Type:    "result",
		IsError: true,
		Result:  "something went wrong",
	}
	data, _ := json.Marshal(envelope)

	_, err := parseSyncResponse(data)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "claude returned error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncRun_PredictionErrorAdjustsTaskPriority(t *testing.T) {
	s := setupTestStore(t)

	seedCompleted := func(notes, tasks int) {
		log, err := s.CreateSyncLog("seed", "prompt")
		if err != nil {
			t.Fatalf("create seed sync log: %v", err)
		}
		if err := s.UpdateSyncLog(log.ID, model.SyncCompleted, "seed", notes, tasks, "", 10, ""); err != nil {
			t.Fatalf("update seed sync log: %v", err)
		}
	}
	seedCompleted(2, 1)
	seedCompleted(2, 1)

	taskA, _ := s.CreateTask("A", "", nil, 1)
	taskB, _ := s.CreateTask("B", "", nil, 0)

	syncResult := SyncResult{
		Summary:        "Prediction run",
		NotesAdded:     4,
		TasksAdded:     5,
		KBFilesUpdated: []string{"ops/prediction.md"},
	}
	envelope := claudeJSONResponse{Type: "result", StructuredOutput: &syncResult}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}

	if !almostEqualFloat(result.PredictedTasks, 1.0) {
		t.Fatalf("expected predicted tasks 1.0, got %.2f", result.PredictedTasks)
	}
	if result.PriorityDelta != 2 {
		t.Fatalf("expected priority delta +2, got %d", result.PriorityDelta)
	}
	if result.AdjustedTasks != 2 {
		t.Fatalf("expected 2 adjusted tasks, got %d", result.AdjustedTasks)
	}

	updatedA, _ := s.GetTask(taskA.ID)
	updatedB, _ := s.GetTask(taskB.ID)
	if updatedA.Priority != 3 {
		t.Fatalf("expected task A priority 3, got %d", updatedA.Priority)
	}
	if updatedB.Priority != 2 {
		t.Fatalf("expected task B priority 2, got %d", updatedB.Priority)
	}

	logs, err := s.ListPredictionErrors(4)
	if err != nil {
		t.Fatalf("list prediction errors: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 prediction logs, got %d", len(logs))
	}

	foundTaskMetric := false
	for _, log := range logs {
		if log.Source != model.PredictionSourceSync {
			continue
		}
		if log.Metric == "tasks_added" {
			foundTaskMetric = true
			if log.PriorityDelta != 2 {
				t.Fatalf("expected tasks_added priority delta 2, got %d", log.PriorityDelta)
			}
		}
	}
	if !foundTaskMetric {
		t.Fatal("expected tasks_added prediction error log")
	}
}

func TestSyncRun_PredictionLearningAdjustsOnlyContextMatchedTasks(t *testing.T) {
	s := setupTestStore(t)

	seedCompleted := func(notes, tasks int) {
		log, err := s.CreateSyncLog("seed", "prompt")
		if err != nil {
			t.Fatalf("create seed sync log: %v", err)
		}
		if err := s.UpdateSyncLog(log.ID, model.SyncCompleted, "seed", notes, tasks, "", 10, ""); err != nil {
			t.Fatalf("update seed sync log: %v", err)
		}
	}
	seedCompleted(2, 1)
	seedCompleted(2, 1)

	if _, err := s.CreateNote("orion release checklist", nil, []string{"orion"}, "manual"); err != nil {
		t.Fatalf("create context note: %v", err)
	}
	matchedTask, _ := s.CreateTask("Prepare Orion rollout", "", nil, 1)
	otherTask, _ := s.CreateTask("Refactor payroll parser", "", nil, 1)

	syncResult := SyncResult{
		Summary:        "Prediction run with context",
		NotesAdded:     3,
		TasksAdded:     5,
		KBFilesUpdated: []string{"ops/context.md"},
	}
	envelope := claudeJSONResponse{Type: "result", StructuredOutput: &syncResult}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.PriorityDelta != 2 {
		t.Fatalf("expected priority delta +2, got %d", result.PriorityDelta)
	}
	if result.AdjustedTasks != 1 {
		t.Fatalf("expected only 1 adjusted task, got %d", result.AdjustedTasks)
	}

	afterMatched, _ := s.GetTask(matchedTask.ID)
	afterOther, _ := s.GetTask(otherTask.ID)
	if afterMatched.Priority != 3 {
		t.Fatalf("expected matched task priority 3, got %d", afterMatched.Priority)
	}
	if afterOther.Priority != 1 {
		t.Fatalf("expected unmatched task priority to remain 1, got %d", afterOther.Priority)
	}
}

func TestPriorityDeltaFromError(t *testing.T) {
	cases := []struct {
		err  float64
		want int
	}{
		{3.1, 2},
		{1.0, 1},
		{0.1, 0},
		{-0.9, 0},
		{-1.2, -1},
		{-4.0, -2},
	}
	for _, c := range cases {
		got := priorityDeltaFromError(c.err)
		if got != c.want {
			t.Fatalf("priorityDeltaFromError(%f)=%d want %d", c.err, got, c.want)
		}
	}
}

func TestSyncRun_PredictionLearningDisabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "0")

	task, _ := s.CreateTask("task", "", nil, 1)

	syncResult := SyncResult{
		Summary:    "disabled prediction",
		NotesAdded: 1,
		TasksAdded: 4,
	}
	envelope := claudeJSONResponse{Type: "result", StructuredOutput: &syncResult}
	envelopeJSON, _ := json.Marshal(envelope)

	svc := NewService(s, &mockExecutor{output: envelopeJSON}, "")
	result, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("sync run: %v", err)
	}
	if result.PriorityDelta != 0 || result.AdjustedTasks != 0 {
		t.Fatalf("priority learning should be disabled, got delta=%d adjusted=%d", result.PriorityDelta, result.AdjustedTasks)
	}

	afterTask, _ := s.GetTask(task.ID)
	if afterTask.Priority != 1 {
		t.Fatalf("task priority should not change when disabled, got %d", afterTask.Priority)
	}

	logs, err := s.ListPredictionErrors(5)
	if err != nil {
		t.Fatalf("list prediction logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no prediction logs when disabled, got %d", len(logs))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func almostEqualFloat(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}
