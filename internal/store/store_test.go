package store

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestMigrateV4AddsMemoryColumns(t *testing.T) {
	s := setupTestStore(t)

	cols, err := noteColumns(s.db)
	if err != nil {
		t.Fatalf("load note columns: %v", err)
	}

	required := []string{
		"strength",
		"decay_rate",
		"salience",
		"recall_count",
		"last_recalled_at",
	}
	for _, col := range required {
		if _, ok := cols[col]; !ok {
			t.Fatalf("missing column %q in notes table", col)
		}
	}
}

func TestMigrateFromV3ToV4BackfillsDefaults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	createSchemaV3Database(t, dbPath)

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 4 {
		t.Fatalf("expected schema version 4, got %d", version)
	}

	var strength, decayRate, salience float64
	var recallCount int
	var lastRecalledAt sql.NullString
	err = s.db.QueryRow(
		`SELECT strength, decay_rate, salience, recall_count, last_recalled_at FROM notes LIMIT 1`,
	).Scan(&strength, &decayRate, &salience, &recallCount, &lastRecalledAt)
	if err != nil {
		t.Fatalf("read migrated note defaults: %v", err)
	}

	if !almostEqual(strength, 0.30) {
		t.Fatalf("strength default mismatch: got %f", strength)
	}
	if !almostEqual(decayRate, 0.015) {
		t.Fatalf("decay_rate default mismatch: got %f", decayRate)
	}
	if !almostEqual(salience, 0.50) {
		t.Fatalf("salience default mismatch: got %f", salience)
	}
	if recallCount != 0 {
		t.Fatalf("recall_count default mismatch: got %d", recallCount)
	}
	if lastRecalledAt.Valid {
		t.Fatalf("expected last_recalled_at NULL, got %q", lastRecalledAt.String)
	}
}

func TestRecallNote(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("important memo", nil, []string{"brain"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	before, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note before recall: %v", err)
	}

	recallAt := before.CreatedAt.Add(2 * time.Hour)
	if err := s.RecallNote(note.ID, recallAt, "unit-test"); err != nil {
		t.Fatalf("recall note: %v", err)
	}

	after, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after recall: %v", err)
	}
	if after.Strength <= before.Strength {
		t.Fatalf("expected strength to increase, before=%f after=%f", before.Strength, after.Strength)
	}
	if after.RecallCount != before.RecallCount+1 {
		t.Fatalf("expected recall_count %d, got %d", before.RecallCount+1, after.RecallCount)
	}
	if after.LastRecalledAt == nil {
		t.Fatal("expected last_recalled_at to be set")
	}
	if !after.LastRecalledAt.Equal(recallAt.UTC()) {
		t.Fatalf("last_recalled_at mismatch: got %s want %s", after.LastRecalledAt.UTC(), recallAt.UTC())
	}
}

func TestRecallNoteNotFound(t *testing.T) {
	s := setupTestStore(t)
	if err := s.RecallNote(9999, time.Now(), "missing"); err == nil {
		t.Fatal("expected error for missing note")
	}
}

func TestDecayMemories(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("decay target", nil, nil, "sync")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	base := time.Now().UTC().Add(-10 * 24 * time.Hour)
	baseStr := base.Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE notes SET strength = ?, decay_rate = ?, last_recalled_at = ?, updated_at = ? WHERE id = ?`,
		0.90, 0.10, baseStr, baseStr, note.ID,
	); err != nil {
		t.Fatalf("seed decay data: %v", err)
	}

	affected, err := s.DecayMemories(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay memories: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 affected note, got %d", affected)
	}

	after, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after decay: %v", err)
	}
	if after.Strength >= 0.90 {
		t.Fatalf("expected strength to decay below 0.90, got %f", after.Strength)
	}
	if after.Strength < minStrength {
		t.Fatalf("strength dropped below minStrength: %f", after.Strength)
	}
}

func TestDecayMemoriesRespectsMinStrength(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("min bound", nil, nil, "sync")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	base := time.Now().UTC().Add(-365 * 24 * time.Hour)
	baseStr := base.Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE notes SET strength = ?, decay_rate = ?, last_recalled_at = ?, updated_at = ? WHERE id = ?`,
		0.06, 0.50, baseStr, baseStr, note.ID,
	); err != nil {
		t.Fatalf("seed min-bound decay data: %v", err)
	}

	_, err = s.DecayMemories(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay memories: %v", err)
	}

	after, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after decay: %v", err)
	}
	if !almostEqual(after.Strength, minStrength) {
		t.Fatalf("expected strength=%f, got %f", minStrength, after.Strength)
	}
}

func TestDecayMemoriesUsesMostRecentMemoryTimestamp(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("decay cadence", nil, nil, "sync")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	decayRate := 0.10
	startStrength := 0.90
	base := time.Now().UTC().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	baseStr := base.Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE notes SET strength = ?, decay_rate = ?, last_recalled_at = ?, updated_at = ? WHERE id = ?`,
		startStrength, decayRate, baseStr, baseStr, note.ID,
	); err != nil {
		t.Fatalf("seed decay cadence data: %v", err)
	}

	firstRunAt := base.Add(10 * 24 * time.Hour)
	if _, err := s.DecayMemories(firstRunAt); err != nil {
		t.Fatalf("first decay: %v", err)
	}
	afterFirst, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after first decay: %v", err)
	}

	secondRunAt := firstRunAt.Add(24 * time.Hour)
	if _, err := s.DecayMemories(secondRunAt); err != nil {
		t.Fatalf("second decay: %v", err)
	}
	afterSecond, err := s.GetNote(note.ID)
	if err != nil {
		t.Fatalf("get note after second decay: %v", err)
	}

	expectedSecond := afterFirst.Strength * math.Exp(-decayRate*1.0)
	if !almostEqual(afterSecond.Strength, expectedSecond) {
		t.Fatalf("unexpected second decay strength: got %f want %f", afterSecond.Strength, expectedSecond)
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
	if note.Strength <= 0 || note.DecayRate <= 0 || note.Salience <= 0 {
		t.Fatalf("expected memory fields to be initialized, got strength=%f decay_rate=%f salience=%f", note.Strength, note.DecayRate, note.Salience)
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

func noteColumns(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`PRAGMA table_info(notes)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultVal sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}
		result[name] = struct{}{}
	}
	return result, rows.Err()
}

func createSchemaV3Database(t *testing.T, dbPath string) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	if err := migrateV1(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrate v1: %v", err)
	}
	if err := migrateV2(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrate v2: %v", err)
	}
	if err := migrateV3(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrate v3: %v", err)
	}
	if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
		tx.Rollback()
		t.Fatalf("clear schema_version: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (3)`); err != nil {
		tx.Rollback()
		t.Fatalf("insert schema version: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v3 schema: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		`INSERT INTO sessions (title, goal, status, started_at, created_at, updated_at) VALUES (?, ?, 'completed', ?, ?, ?)`,
		"legacy session", "migrate", now, now, now,
	)
	if err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO notes (session_id, content, tags, source, created_at, updated_at, consolidated_at) VALUES (?, ?, '', '', ?, ?, NULL)`,
		sessionID, "legacy note", now, now,
	); err != nil {
		t.Fatalf("insert legacy note: %v", err)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
