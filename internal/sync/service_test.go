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
