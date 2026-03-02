package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/kb"
	sbmcp "github.com/urugus/second-brain/internal/mcp"
	"github.com/urugus/second-brain/internal/store"
)

// mockAgent implements adapter.Agent for MCP consolidation tests.
type mockAgent struct {
	result *adapter.ConsolidationResult
	err    error
}

func (m *mockAgent) Name() string { return "mock" }

func (m *mockAgent) Consolidate(_ context.Context, _ adapter.ConsolidationRequest) (*adapter.ConsolidationResult, error) {
	return m.result, m.err
}

func (m *mockAgent) Summarize(_ context.Context, text string) (string, error) {
	return "summary", nil
}

func setup(t *testing.T) (*gomcp.ClientSession, *store.Store, *kb.KB) {
	t.Helper()
	return setupWithOpts(t)
}

func setupWithMock(t *testing.T, agent *mockAgent) (*gomcp.ClientSession, *store.Store, *kb.KB) {
	t.Helper()
	factory := func(_ string) adapter.Agent { return agent }
	return setupWithOpts(t, sbmcp.WithAgentFactory(factory))
}

func setupWithOpts(t *testing.T, opts ...sbmcp.Option) (*gomcp.ClientSession, *store.Store, *kb.KB) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	k := kb.New(filepath.Join(dir, "knowledge"))

	server := sbmcp.New(s, k, opts...)
	serverTransport, clientTransport := gomcp.NewInMemoryTransports()

	ctx := context.Background()
	go server.Run(ctx, serverTransport)

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session, s, k
}

func callTool(t *testing.T, session *gomcp.ClientSession, name string, args any) *gomcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s failed: %v", name, err)
	}
	return result
}

func getTextContent(t *testing.T, result *gomcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty content in result")
	}
	data, err := result.Content[0].MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var tc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &tc); err != nil {
		t.Fatal(err)
	}
	return tc.Text
}

func TestServerInstructions(t *testing.T) {
	session, _, _ := setup(t)
	result := session.InitializeResult()
	if result == nil {
		t.Fatal("expected non-nil InitializeResult")
	}
	if result.Instructions == "" {
		t.Fatal("expected non-empty server instructions")
	}
	// Verify instructions mention key concepts
	for _, keyword := range []string{"second-brain", "session", "task", "note", "knowledge base", "consolidate"} {
		if !strings.Contains(result.Instructions, keyword) {
			t.Errorf("instructions should mention %q", keyword)
		}
	}
}

func TestGetActiveSession_NoSession(t *testing.T) {
	session, _, _ := setup(t)
	result := callTool(t, session, "get_active_session", nil)
	text := getTextContent(t, result)
	if text != "No active session" {
		t.Errorf("expected 'No active session', got %q", text)
	}
}

func TestGetActiveSession_WithSession(t *testing.T) {
	session, s, _ := setup(t)
	_, err := s.CreateSession("Test Session", "Test goal")
	if err != nil {
		t.Fatal(err)
	}

	result := callTool(t, session, "get_active_session", nil)
	text := getTextContent(t, result)

	var sess map[string]any
	if err := json.Unmarshal([]byte(text), &sess); err != nil {
		t.Fatalf("failed to parse session JSON: %v", err)
	}
	if sess["Title"] != "Test Session" {
		t.Errorf("expected title 'Test Session', got %v", sess["Title"])
	}
}

func TestListSessions(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Session 1", "")
	s.EndSession(1, "done")
	s.CreateSession("Session 2", "")

	result := callTool(t, session, "list_sessions", nil)
	text := getTextContent(t, result)

	var sessions []map[string]any
	if err := json.Unmarshal([]byte(text), &sessions); err != nil {
		t.Fatalf("failed to parse sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestListSessions_FilterByStatus(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Session 1", "")
	s.EndSession(1, "done")
	s.CreateSession("Session 2", "")

	result := callTool(t, session, "list_sessions", map[string]any{"status": "active"})
	text := getTextContent(t, result)

	var sessions []map[string]any
	if err := json.Unmarshal([]byte(text), &sessions); err != nil {
		t.Fatalf("failed to parse sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 active session, got %d", len(sessions))
	}
}

func TestCreateTask(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Work Session", "")

	result := callTool(t, session, "create_task", map[string]any{
		"title":       "Test Task",
		"description": "Task description",
		"priority":    2,
	})
	text := getTextContent(t, result)

	var task map[string]any
	if err := json.Unmarshal([]byte(text), &task); err != nil {
		t.Fatalf("failed to parse task: %v", err)
	}
	if task["Title"] != "Test Task" {
		t.Errorf("expected title 'Test Task', got %v", task["Title"])
	}
	// Verify auto-attached to active session
	if task["SessionID"] == nil {
		t.Error("expected task to be attached to active session")
	}
}

func TestListTasks(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateTask("Task 1", "", nil, 0)
	s.CreateTask("Task 2", "", nil, 0)

	result := callTool(t, session, "list_tasks", nil)
	text := getTextContent(t, result)

	var tasks []map[string]any
	if err := json.Unmarshal([]byte(text), &tasks); err != nil {
		t.Fatalf("failed to parse tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateTask("Task 1", "", nil, 0)

	result := callTool(t, session, "update_task_status", map[string]any{
		"id":     1,
		"status": "done",
	})
	text := getTextContent(t, result)
	if text != "Task 1 status updated to done" {
		t.Errorf("unexpected response: %q", text)
	}

	// Verify via store
	task, _ := s.GetTask(1)
	if task.Status != "done" {
		t.Errorf("expected status done, got %s", task.Status)
	}
}

func TestCreateNote(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Work Session", "")

	result := callTool(t, session, "create_note", map[string]any{
		"content": "Important note",
		"tags":    []string{"learning", "go"},
		"source":  "claude-code",
	})
	text := getTextContent(t, result)

	var note map[string]any
	if err := json.Unmarshal([]byte(text), &note); err != nil {
		t.Fatalf("failed to parse note: %v", err)
	}
	if note["Content"] != "Important note" {
		t.Errorf("expected content 'Important note', got %v", note["Content"])
	}
}

func TestListNotes(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateNote("Note 1", nil, []string{"tag1"}, "")
	s.CreateNote("Note 2", nil, []string{"tag2"}, "")

	result := callTool(t, session, "list_notes", nil)
	text := getTextContent(t, result)

	var notes []map[string]any
	if err := json.Unmarshal([]byte(text), &notes); err != nil {
		t.Fatalf("failed to parse notes: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
}

func TestKBListEmpty(t *testing.T) {
	session, _, _ := setup(t)
	result := callTool(t, session, "kb_list", nil)
	text := getTextContent(t, result)
	if text != "No KB files found" {
		t.Errorf("expected 'No KB files found', got %q", text)
	}
}

func TestKBWriteAndRead(t *testing.T) {
	session, _, k := setup(t)

	// Write
	result := callTool(t, session, "kb_write", map[string]any{
		"path":    "test/hello.md",
		"content": "# Hello\nWorld",
	})
	text := getTextContent(t, result)
	if text != `KB file "test/hello.md" written successfully` {
		t.Errorf("unexpected write result: %q", text)
	}

	// Verify via KB directly
	if !k.Exists("test/hello.md") {
		t.Error("KB file should exist")
	}

	// Read via MCP
	result = callTool(t, session, "kb_read", map[string]any{"path": "test/hello.md"})
	text = getTextContent(t, result)
	if text != "# Hello\nWorld" {
		t.Errorf("expected '# Hello\\nWorld', got %q", text)
	}

	// List
	result = callTool(t, session, "kb_list", nil)
	text = getTextContent(t, result)
	var files []string
	if err := json.Unmarshal([]byte(text), &files); err != nil {
		t.Fatalf("failed to parse files: %v", err)
	}
	if len(files) != 1 || files[0] != "test/hello.md" {
		t.Errorf("expected [test/hello.md], got %v", files)
	}
}

func TestKBSearch(t *testing.T) {
	session, _, _ := setup(t)

	// Write some files first
	callTool(t, session, "kb_write", map[string]any{
		"path":    "go-tips.md",
		"content": "# Go Tips\nUse interfaces for abstraction",
	})

	result := callTool(t, session, "kb_search", map[string]any{"query": "interfaces"})
	text := getTextContent(t, result)

	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("failed to parse search results: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestListEvents(t *testing.T) {
	session, s, _ := setup(t)
	sess, _ := s.CreateSession("Test", "")
	s.CreateTask("Task 1", "", &sess.ID, 0)

	result := callTool(t, session, "list_events", map[string]any{"session_id": sess.ID})
	text := getTextContent(t, result)

	var events []map[string]any
	if err := json.Unmarshal([]byte(text), &events); err != nil {
		t.Fatalf("failed to parse events: %v", err)
	}
	// At least session.started and task.created
	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}
}

func TestStartSession(t *testing.T) {
	session, _, _ := setup(t)

	result := callTool(t, session, "start_session", map[string]any{
		"title": "Dev Session",
		"goal":  "Implement feature X",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getTextContent(t, result))
	}
	text := getTextContent(t, result)

	var sess map[string]any
	if err := json.Unmarshal([]byte(text), &sess); err != nil {
		t.Fatalf("failed to parse session JSON: %v", err)
	}
	if sess["Title"] != "Dev Session" {
		t.Errorf("expected title 'Dev Session', got %v", sess["Title"])
	}
	if sess["Goal"] != "Implement feature X" {
		t.Errorf("expected goal 'Implement feature X', got %v", sess["Goal"])
	}
	if sess["Status"] != "active" {
		t.Errorf("expected status 'active', got %v", sess["Status"])
	}
}

func TestStartSession_FailWhenActiveExists(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Existing Session", "")

	result := callTool(t, session, "start_session", map[string]any{
		"title": "Another Session",
	})
	if !result.IsError {
		t.Error("expected error when active session exists")
	}
	text := getTextContent(t, result)
	if text != "failed to start session: an active session already exists; end or abandon it first" {
		t.Errorf("unexpected error message: %q", text)
	}
}

func TestEndSession(t *testing.T) {
	session, s, _ := setup(t)
	s.CreateSession("Work Session", "some goal")

	result := callTool(t, session, "end_session", map[string]any{
		"summary": "Completed the work",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getTextContent(t, result))
	}
	text := getTextContent(t, result)

	var sess map[string]any
	if err := json.Unmarshal([]byte(text), &sess); err != nil {
		t.Fatalf("failed to parse session JSON: %v", err)
	}
	if sess["Title"] != "Work Session" {
		t.Errorf("expected title 'Work Session', got %v", sess["Title"])
	}
	if sess["Status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", sess["Status"])
	}
	if sess["Summary"] != "Completed the work" {
		t.Errorf("expected summary 'Completed the work', got %v", sess["Summary"])
	}
}

func TestEndSession_FailWhenNoActive(t *testing.T) {
	session, _, _ := setup(t)

	result := callTool(t, session, "end_session", map[string]any{
		"summary": "Nothing to end",
	})
	if !result.IsError {
		t.Error("expected error when no active session")
	}
	text := getTextContent(t, result)
	if text != "no active session" {
		t.Errorf("unexpected error message: %q", text)
	}
}

func TestToolsListCount(t *testing.T) {
	session, _, _ := setup(t)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 15 {
		t.Errorf("expected 15 tools, got %d", len(result.Tools))
		for _, tool := range result.Tools {
			t.Logf("  - %s", tool.Name)
		}
	}
}

func TestConsolidate_ProposeMode(t *testing.T) {
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
	session, s, _ := setupWithMock(t, agent)

	// Create and complete a session
	sess, _ := s.CreateSession("Test Session", "test goal")
	s.CreateTask("Task 1", "do something", &sess.ID, 1)
	s.CreateNote("interesting finding", &sess.ID, []string{"research"}, "manual")
	s.EndSession(sess.ID, "completed work")

	result := callTool(t, session, "consolidate", map[string]any{
		"session_id": sess.ID,
		"mode":       "propose",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getTextContent(t, result))
	}
	text := getTextContent(t, result)

	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "proposed" {
		t.Errorf("expected status 'proposed', got %v", resp["status"])
	}
	if resp["summary"] != "Session focused on testing." {
		t.Errorf("unexpected summary: %v", resp["summary"])
	}
	kbUpdates, ok := resp["kb_updates"].([]any)
	if !ok || len(kbUpdates) != 1 {
		t.Errorf("expected 1 KB update, got %v", resp["kb_updates"])
	}
	suggestedTasks, ok := resp["suggested_tasks"].([]any)
	if !ok || len(suggestedTasks) != 1 {
		t.Errorf("expected 1 suggested task, got %v", resp["suggested_tasks"])
	}
}

func TestConsolidate_ApplyMode(t *testing.T) {
	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary: "Applied session changes.",
			KBUpdates: []adapter.KBUpdate{
				{
					Path:    "notes/session.md",
					Content: "# Session Notes\n\nKey findings.\n",
					Reason:  "Capture session notes",
				},
			},
			SuggestedTasks: []string{"Follow up on findings"},
		},
	}
	session, s, k := setupWithMock(t, agent)

	sess, _ := s.CreateSession("Apply Session", "apply goal")
	s.EndSession(sess.ID, "done")

	result := callTool(t, session, "consolidate", map[string]any{
		"session_id": sess.ID,
		"mode":       "apply",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getTextContent(t, result))
	}
	text := getTextContent(t, result)

	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "applied" {
		t.Errorf("expected status 'applied', got %v", resp["status"])
	}

	// Verify KB file was written
	if !k.Exists("notes/session.md") {
		t.Error("expected KB file notes/session.md to exist after apply")
	}
	content, err := k.Read("notes/session.md")
	if err != nil {
		t.Fatalf("failed to read KB file: %v", err)
	}
	if content != "# Session Notes\n\nKey findings.\n" {
		t.Errorf("unexpected KB content: %q", content)
	}

	// Verify task was created
	tasks, _ := s.ListTasks(store.TaskFilter{})
	found := false
	for _, task := range tasks {
		if task.Title == "Follow up on findings" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected suggested task to be created after apply")
	}
}

func TestConsolidate_LatestUnconsolidated(t *testing.T) {
	agent := &mockAgent{
		result: &adapter.ConsolidationResult{
			Summary:   "Auto-found session.",
			KBUpdates: []adapter.KBUpdate{},
		},
	}
	session, s, _ := setupWithMock(t, agent)

	// Create and complete a session (don't specify session_id)
	sess, _ := s.CreateSession("Auto Session", "")
	s.EndSession(sess.ID, "done")

	result := callTool(t, session, "consolidate", map[string]any{
		"mode": "propose",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getTextContent(t, result))
	}
	text := getTextContent(t, result)

	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Should have found the session automatically
	sessionID := resp["session_id"].(float64)
	if int64(sessionID) != sess.ID {
		t.Errorf("expected session_id %d, got %v", sess.ID, sessionID)
	}
}

func TestConsolidate_InvalidMode(t *testing.T) {
	session, _, _ := setup(t)

	result := callTool(t, session, "consolidate", map[string]any{
		"mode": "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid mode")
	}
	text := getTextContent(t, result)
	if text != `invalid mode "invalid": must be 'propose' or 'apply'` {
		t.Errorf("unexpected error message: %q", text)
	}
}

func TestConsolidate_NoUnconsolidatedSessions(t *testing.T) {
	session, _, _ := setup(t)

	result := callTool(t, session, "consolidate", nil)
	if !result.IsError {
		t.Error("expected error when no sessions available")
	}
	text := getTextContent(t, result)
	if text != "no unconsolidated sessions found" {
		t.Errorf("unexpected error message: %q", text)
	}
}
