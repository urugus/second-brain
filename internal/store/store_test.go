package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/urugus/second-brain/internal/model"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	// Opening again should be idempotent
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s2.Close()
}

func TestOpenCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "dir", "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := setupTestStore(t)

	// Create session
	sess, err := s.CreateSession("Test Session", "test goal")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID == 0 {
		t.Fatal("expected non-zero session ID")
	}
	if sess.Status != model.SessionActive {
		t.Fatalf("expected active, got %s", sess.Status)
	}

	// Cannot create another active session
	_, err = s.CreateSession("Another", "")
	if err == nil {
		t.Fatal("expected error creating second active session")
	}

	// Get session
	got, err := s.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Title != "Test Session" {
		t.Fatalf("expected 'Test Session', got %q", got.Title)
	}

	// Active session
	active, err := s.ActiveSession()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if active == nil || active.ID != sess.ID {
		t.Fatal("expected active session")
	}

	// End session
	if err := s.EndSession(sess.ID, "done"); err != nil {
		t.Fatalf("end session: %v", err)
	}

	got, _ = s.GetSession(sess.ID)
	if got.Status != model.SessionCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.EndedAt == nil {
		t.Fatal("expected ended_at to be set")
	}

	// No active session now
	active, err = s.ActiveSession()
	if err != nil {
		t.Fatalf("active session after end: %v", err)
	}
	if active != nil {
		t.Fatal("expected no active session")
	}

	// Can create new session after ending
	sess2, err := s.CreateSession("Session 2", "")
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}

	// Abandon it
	if err := s.AbandonSession(sess2.ID); err != nil {
		t.Fatalf("abandon session: %v", err)
	}
	got2, _ := s.GetSession(sess2.ID)
	if got2.Status != model.SessionAbandoned {
		t.Fatalf("expected abandoned, got %s", got2.Status)
	}
}

func TestListSessions(t *testing.T) {
	s := setupTestStore(t)

	s.CreateSession("S1", "")
	s.EndSession(1, "")
	s.CreateSession("S2", "")

	all, err := s.ListSessions(nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}

	active := model.SessionActive
	activeList, err := s.ListSessions(&active)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(activeList) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(activeList))
	}
}

func TestTaskCRUD(t *testing.T) {
	s := setupTestStore(t)

	sess, _ := s.CreateSession("Work", "")

	// Create task with session
	task, err := s.CreateTask("Fix bug", "fix the bug", &sess.ID, 2)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != model.TaskTodo {
		t.Fatalf("expected todo, got %s", task.Status)
	}

	// Create task without session
	task2, err := s.CreateTask("Standalone", "", nil, 0)
	if err != nil {
		t.Fatalf("create standalone task: %v", err)
	}
	if task2.SessionID != nil {
		t.Fatal("expected nil session_id")
	}

	// Update status
	if err := s.UpdateTaskStatus(task.ID, model.TaskDone); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ := s.GetTask(task.ID)
	if got.Status != model.TaskDone {
		t.Fatalf("expected done, got %s", got.Status)
	}

	// Update fields
	newTitle := "Fixed bug"
	if err := s.UpdateTask(task.ID, &newTitle, nil, nil); err != nil {
		t.Fatalf("update task: %v", err)
	}
	got, _ = s.GetTask(task.ID)
	if got.Title != "Fixed bug" {
		t.Fatalf("expected 'Fixed bug', got %q", got.Title)
	}

	// List by session
	tasks, err := s.ListTasks(TaskFilter{SessionID: &sess.ID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in session, got %d", len(tasks))
	}

	// List all
	allTasks, err := s.ListTasks(TaskFilter{})
	if err != nil {
		t.Fatalf("list all tasks: %v", err)
	}
	if len(allTasks) != 2 {
		t.Fatalf("expected 2 tasks total, got %d", len(allTasks))
	}
}

func TestNoteCRUD(t *testing.T) {
	s := setupTestStore(t)

	sess, _ := s.CreateSession("Research", "")

	// Create note with session
	note, err := s.CreateNote("interesting finding", &sess.ID, []string{"research", "api"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if len(note.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(note.Tags))
	}

	// Create note without session
	_, err = s.CreateNote("standalone note", nil, nil, "")
	if err != nil {
		t.Fatalf("create standalone note: %v", err)
	}

	// Get note
	got, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if got.Content != "interesting finding" {
		t.Fatalf("expected content, got %q", got.Content)
	}

	// List by session
	notes, err := s.ListNotes(NoteFilter{SessionID: &sess.ID})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note in session, got %d", len(notes))
	}

	// List by tag
	tag := "research"
	tagged, err := s.ListNotes(NoteFilter{Tag: &tag})
	if err != nil {
		t.Fatalf("list by tag: %v", err)
	}
	if len(tagged) != 1 {
		t.Fatalf("expected 1 tagged note, got %d", len(tagged))
	}

	// Non-existing tag
	noTag := "nonexistent"
	empty, err := s.ListNotes(NoteFilter{Tag: &noTag})
	if err != nil {
		t.Fatalf("list by nonexistent tag: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(empty))
	}
}

func TestEventRecording(t *testing.T) {
	s := setupTestStore(t)

	sess, _ := s.CreateSession("Event Test", "test events")
	s.CreateTask("Task 1", "", &sess.ID, 0)
	s.CreateNote("Note 1", &sess.ID, []string{"tag"}, "")
	s.UpdateTaskStatus(1, model.TaskDone)
	s.EndSession(sess.ID, "done")

	events, err := s.ListEventsBySession(sess.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	// Expect: session.started, task.created, note.added, task.status_changed, session.ended
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	expectedTypes := []model.EventType{
		model.EventSessionStarted,
		model.EventTaskCreated,
		model.EventNoteAdded,
		model.EventTaskStatusChanged,
		model.EventSessionEnded,
	}
	for i, et := range expectedTypes {
		if events[i].Type != et {
			t.Errorf("event %d: expected %s, got %s", i, et, events[i].Type)
		}
	}
}
