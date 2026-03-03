package store

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "1")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "1")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "0.18")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "0.35")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_WEIGHT", "0.20")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE", "0.34")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_CANDIDATES", "80")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS", "3")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_ALPHA", "0.12")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_DECAY", "0.05")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_MAX_EDGES", "10")
	t.Setenv("SB_ENTITY_AUTOEDGE_MAX_PAIRS", "20")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_WEIGHT", "0.14")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MAX_LINKS", "4")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MIN_SHARED", "1")
	t.Setenv("SB_ENTITY_FEEDBACK_ALPHA", "0.10")
	t.Setenv("SB_ENTITY_FEEDBACK_DECAY", "0.04")
	t.Setenv("SB_ENTITY_FEEDBACK_MAX_ENTITIES", "10")
	t.Setenv("SB_TASK_PRIORITY_MAX", "5")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "5")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "5")
	t.Setenv("SB_SLEEP_THRESHOLD", "10")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK", "0")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_FEEDBACK", "1")
	t.Setenv("SB_FEATURE_ENTITY_LEARNING", "1")
	t.Setenv("SB_FEATURE_ENTITY_DERIVED_EDGE", "1")
	t.Setenv("SB_FEATURE_ENTITY_FEEDBACK", "1")
	t.Setenv("SB_METRICS_WINDOW_DAYS", "14")

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

func TestMigrateFromV3ToLatestBackfillsDefaults(t *testing.T) {
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
	if version != 9 {
		t.Fatalf("expected schema version 9, got %d", version)
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

func TestMigrateV5CreatesMemoryEdgesTable(t *testing.T) {
	s := setupTestStore(t)

	exists, err := tableExists(s.db, "memory_edges")
	if err != nil {
		t.Fatalf("check table exists: %v", err)
	}
	if !exists {
		t.Fatal("expected memory_edges table to exist")
	}
}

func TestMigrateV6CreatesPredictionErrorLogTable(t *testing.T) {
	s := setupTestStore(t)

	exists, err := tableExists(s.db, "prediction_error_log")
	if err != nil {
		t.Fatalf("check table exists: %v", err)
	}
	if !exists {
		t.Fatal("expected prediction_error_log table to exist")
	}
}

func TestMigrateV8CreatesNotesCreatedAtIndex(t *testing.T) {
	s := setupTestStore(t)

	exists, err := indexExists(s.db, "idx_notes_created_at")
	if err != nil {
		t.Fatalf("check index exists: %v", err)
	}
	if !exists {
		t.Fatal("expected idx_notes_created_at index to exist")
	}
}

func TestMigrateV9CreatesEntityTables(t *testing.T) {
	s := setupTestStore(t)

	for _, table := range []string{"entities", "entity_aliases", "note_entities", "entity_edges"} {
		exists, err := tableExists(s.db, table)
		if err != nil {
			t.Fatalf("check %s exists: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected %s table to exist", table)
		}
	}
}

func TestEstimateSyncPredictionUsesRecentCompletedLogs(t *testing.T) {
	s := setupTestStore(t)

	createLog := func(status model.SyncStatus, notes, tasks int) {
		log, err := s.CreateSyncLog("test-agent", "prompt")
		if err != nil {
			t.Fatalf("create sync log: %v", err)
		}
		if err := s.UpdateSyncLog(log.ID, status, "", notes, tasks, "", 0, ""); err != nil {
			t.Fatalf("update sync log: %v", err)
		}
	}

	createLog(model.SyncCompleted, 1, 1)
	createLog(model.SyncFailed, 100, 100)
	createLog(model.SyncCompleted, 3, 2)
	createLog(model.SyncCompleted, 5, 4)

	predNotes, predTasks, err := s.EstimateSyncPrediction(2)
	if err != nil {
		t.Fatalf("estimate sync prediction: %v", err)
	}
	if !almostEqual(predNotes, 4.0) {
		t.Fatalf("unexpected notes prediction: got %f want 4.0", predNotes)
	}
	if !almostEqual(predTasks, 3.0) {
		t.Fatalf("unexpected tasks prediction: got %f want 3.0", predTasks)
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

func TestRecallNote_ContextMatchBoostsStrength(t *testing.T) {
	s := setupTestStore(t)

	matched, err := s.CreateNote("golang interfaces and adapters", nil, []string{"go"}, "manual")
	if err != nil {
		t.Fatalf("create matched note: %v", err)
	}
	unmatched, err := s.CreateNote("python decorators and closures", nil, []string{"python"}, "manual")
	if err != nil {
		t.Fatalf("create unmatched note: %v", err)
	}

	beforeMatched, err := s.GetNote(matched.ID)
	if err != nil {
		t.Fatalf("get matched note before recall: %v", err)
	}
	beforeUnmatched, err := s.GetNote(unmatched.ID)
	if err != nil {
		t.Fatalf("get unmatched note before recall: %v", err)
	}

	recallAt := beforeMatched.CreatedAt.Add(2 * time.Hour)
	context := "interfaces adapter"
	if err := s.RecallNote(matched.ID, recallAt, context); err != nil {
		t.Fatalf("recall matched note: %v", err)
	}
	if err := s.RecallNote(unmatched.ID, recallAt, context); err != nil {
		t.Fatalf("recall unmatched note: %v", err)
	}

	afterMatched, err := s.GetNote(matched.ID)
	if err != nil {
		t.Fatalf("get matched note after recall: %v", err)
	}
	afterUnmatched, err := s.GetNote(unmatched.ID)
	if err != nil {
		t.Fatalf("get unmatched note after recall: %v", err)
	}

	deltaMatched := afterMatched.Strength - beforeMatched.Strength
	deltaUnmatched := afterUnmatched.Strength - beforeUnmatched.Strength
	if deltaMatched <= deltaUnmatched {
		t.Fatalf(
			"expected context-matched note to gain more strength: matched=%f unmatched=%f",
			deltaMatched,
			deltaUnmatched,
		)
	}
}

func TestRecallNote_FeedbackLearningAdjustsEdgeWeights(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_ALPHA", "0.20")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_DECAY", "0.10")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_MAX_EDGES", "10")

	seed, _ := s.CreateNote("kafka retry strategy", nil, []string{"kafka"}, "manual")
	match, _ := s.CreateNote("kafka backoff tuning", nil, []string{"kafka", "retry"}, "manual")
	miss, _ := s.CreateNote("design token palette", nil, []string{"design"}, "manual")

	if err := s.LinkNotes(seed.ID, match.ID, 0.50, "seed->match"); err != nil {
		t.Fatalf("link seed->match: %v", err)
	}
	if err := s.LinkNotes(seed.ID, miss.ID, 0.50, "seed->miss"); err != nil {
		t.Fatalf("link seed->miss: %v", err)
	}

	if err := s.RecallNote(seed.ID, time.Now().UTC(), "kafka backoff"); err != nil {
		t.Fatalf("recall seed note: %v", err)
	}

	var matchWeight float64
	var matchReinforced int
	if err := s.db.QueryRow(
		`SELECT weight, reinforced_count FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		seed.ID, match.ID,
	).Scan(&matchWeight, &matchReinforced); err != nil {
		t.Fatalf("query seed->match edge: %v", err)
	}
	if matchWeight <= 0.50 {
		t.Fatalf("expected matched edge reinforcement, got weight=%f", matchWeight)
	}
	if matchReinforced != 2 {
		t.Fatalf("expected matched edge reinforced_count=2, got %d", matchReinforced)
	}

	var missWeight float64
	var missReinforced int
	if err := s.db.QueryRow(
		`SELECT weight, reinforced_count FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		seed.ID, miss.ID,
	).Scan(&missWeight, &missReinforced); err != nil {
		t.Fatalf("query seed->miss edge: %v", err)
	}
	if missWeight >= 0.50 {
		t.Fatalf("expected missed edge decay, got weight=%f", missWeight)
	}
	if missReinforced != 1 {
		t.Fatalf("expected missed edge reinforced_count to remain 1, got %d", missReinforced)
	}
}

func TestRecallNote_FeedbackLearningDisabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_MEMORY_EDGE_FEEDBACK", "0")

	seed, _ := s.CreateNote("incident playbook", nil, []string{"ops"}, "manual")
	related, _ := s.CreateNote("incident rollback sequence", nil, []string{"ops"}, "manual")
	if err := s.LinkNotes(seed.ID, related.ID, 0.60, "seed->related"); err != nil {
		t.Fatalf("link seed->related: %v", err)
	}

	if err := s.RecallNote(seed.ID, time.Now().UTC(), "incident rollback"); err != nil {
		t.Fatalf("recall seed note: %v", err)
	}

	var weight float64
	var reinforced int
	if err := s.db.QueryRow(
		`SELECT weight, reinforced_count FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		seed.ID, related.ID,
	).Scan(&weight, &reinforced); err != nil {
		t.Fatalf("query edge: %v", err)
	}
	if !almostEqual(weight, 0.60) {
		t.Fatalf("expected unchanged edge weight when feedback disabled, got %f", weight)
	}
	if reinforced != 1 {
		t.Fatalf("expected unchanged reinforced_count when feedback disabled, got %d", reinforced)
	}
}

func TestRecallNote_EntityFeedbackReinforcesMatchedEntities(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_FEEDBACK_ALPHA", "0.20")

	note, err := s.CreateNote(
		"Grace Hopper compiler retrospective",
		nil,
		[]string{"person:Grace Hopper", "concept:Compiler"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}

	beforeEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before recall: %v", err)
	}
	beforePerson, ok := findEntityByKindAndName(beforeEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found before recall: %+v", beforeEntities)
	}

	if err := s.RecallNote(note.ID, time.Now().UTC(), "grace hopper compiler memo"); err != nil {
		t.Fatalf("recall note: %v", err)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after recall: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found after recall: %+v", afterEntities)
	}

	if afterPerson.Strength <= beforePerson.Strength {
		t.Fatalf("expected person strength reinforcement, before=%f after=%f", beforePerson.Strength, afterPerson.Strength)
	}
	if afterPerson.Salience <= beforePerson.Salience {
		t.Fatalf("expected person salience reinforcement, before=%f after=%f", beforePerson.Salience, afterPerson.Salience)
	}
}

func TestRecallNote_EntityFeedbackDecaysUnmatchedEntities(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_FEEDBACK_DECAY", "0.20")

	note, err := s.CreateNote(
		"Grace Hopper compiler retrospective",
		nil,
		[]string{"person:Grace Hopper"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}

	beforeEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before recall: %v", err)
	}
	beforePerson, ok := findEntityByKindAndName(beforeEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found before recall: %+v", beforeEntities)
	}

	if err := s.RecallNote(note.ID, time.Now().UTC(), "design token typography"); err != nil {
		t.Fatalf("recall note: %v", err)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after recall: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found after recall: %+v", afterEntities)
	}

	if afterPerson.Strength >= beforePerson.Strength {
		t.Fatalf("expected person strength decay on mismatch, before=%f after=%f", beforePerson.Strength, afterPerson.Strength)
	}
	if afterPerson.Strength < minStrength {
		t.Fatalf("expected person strength floor >= %f, got %f", minStrength, afterPerson.Strength)
	}
}

func TestRecallNote_EntityFeedbackDisabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_ENTITY_FEEDBACK", "0")

	note, err := s.CreateNote(
		"Grace Hopper compiler retrospective",
		nil,
		[]string{"person:Grace Hopper"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}

	beforeEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before recall: %v", err)
	}
	beforePerson, ok := findEntityByKindAndName(beforeEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found before recall: %+v", beforeEntities)
	}

	if err := s.RecallNote(note.ID, time.Now().UTC(), "grace hopper compiler"); err != nil {
		t.Fatalf("recall note: %v", err)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after recall: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("person entity not found after recall: %+v", afterEntities)
	}

	if !almostEqual(beforePerson.Strength, afterPerson.Strength) {
		t.Fatalf("expected unchanged entity strength when feedback disabled, before=%f after=%f", beforePerson.Strength, afterPerson.Strength)
	}
	if !almostEqual(beforePerson.Salience, afterPerson.Salience) {
		t.Fatalf("expected unchanged entity salience when feedback disabled, before=%f after=%f", beforePerson.Salience, afterPerson.Salience)
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

func TestApplySleepReplayConsolidation(t *testing.T) {
	s := setupTestStore(t)

	n1, err := s.CreateNote("replay core", nil, nil, "manual")
	if err != nil {
		t.Fatalf("create note 1: %v", err)
	}
	n2, err := s.CreateNote("replay merged", nil, nil, "manual")
	if err != nil {
		t.Fatalf("create note 2: %v", err)
	}

	runAt := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second)
	if err := s.ApplySleepReplayConsolidation(map[int64]float64{
		n1.ID: 1.0,
		n2.ID: 0.35,
	}, runAt); err != nil {
		t.Fatalf("apply sleep replay consolidation: %v", err)
	}

	after1, err := s.GetNote(n1.ID)
	if err != nil {
		t.Fatalf("get note 1: %v", err)
	}
	after2, err := s.GetNote(n2.ID)
	if err != nil {
		t.Fatalf("get note 2: %v", err)
	}

	if after1.ConsolidatedAt == nil || !after1.ConsolidatedAt.Equal(runAt) {
		t.Fatalf("note 1 consolidated_at mismatch: got %v want %v", after1.ConsolidatedAt, runAt)
	}
	if after2.ConsolidatedAt == nil || !after2.ConsolidatedAt.Equal(runAt) {
		t.Fatalf("note 2 consolidated_at mismatch: got %v want %v", after2.ConsolidatedAt, runAt)
	}
	if after1.Strength <= n1.Strength {
		t.Fatalf("note 1 strength should increase: before=%f after=%f", n1.Strength, after1.Strength)
	}
	if after2.Strength <= n2.Strength {
		t.Fatalf("note 2 strength should increase: before=%f after=%f", n2.Strength, after2.Strength)
	}

	delta1 := after1.Strength - n1.Strength
	delta2 := after2.Strength - n2.Strength
	if delta1 <= delta2 {
		t.Fatalf("expected canonical replay delta > merged replay delta, got canonical=%f merged=%f", delta1, delta2)
	}
}

func TestAdjustTodoTaskPriorities(t *testing.T) {
	s := setupTestStore(t)

	t1, _ := s.CreateTask("low", "", nil, 0)
	t2, _ := s.CreateTask("mid", "", nil, 2)
	t3, _ := s.CreateTask("high", "", nil, 4)
	doneTask, _ := s.CreateTask("done", "", nil, 3)
	if err := s.UpdateTaskStatus(doneTask.ID, model.TaskDone); err != nil {
		t.Fatalf("mark done task: %v", err)
	}

	adjusted, err := s.AdjustTodoTaskPriorities(1, 2, nil)
	if err != nil {
		t.Fatalf("adjust todo priorities: %v", err)
	}
	if adjusted != 2 {
		t.Fatalf("expected 2 adjusted tasks, got %d", adjusted)
	}

	after1, _ := s.GetTask(t1.ID)
	after2, _ := s.GetTask(t2.ID)
	after3, _ := s.GetTask(t3.ID)
	afterDone, _ := s.GetTask(doneTask.ID)

	if after3.Priority != 5 {
		t.Fatalf("expected high task priority 5, got %d", after3.Priority)
	}
	if after2.Priority != 3 {
		t.Fatalf("expected mid task priority 3, got %d", after2.Priority)
	}
	if after1.Priority != 0 {
		t.Fatalf("expected low task to remain 0, got %d", after1.Priority)
	}
	if afterDone.Priority != 3 {
		t.Fatalf("expected done task to remain 3, got %d", afterDone.Priority)
	}
}

func TestAdjustTodoTaskPriorities_ContextFiltered(t *testing.T) {
	s := setupTestStore(t)

	matchTask, _ := s.CreateTask("Prepare Orion rollout", "Coordinate release runbook", nil, 2)
	otherTask, _ := s.CreateTask("Refactor payroll parser", "cleanup internals", nil, 2)

	adjusted, err := s.AdjustTodoTaskPriorities(1, 5, []string{"orion"})
	if err != nil {
		t.Fatalf("adjust todo priorities with context: %v", err)
	}
	if adjusted != 1 {
		t.Fatalf("expected 1 adjusted task, got %d", adjusted)
	}

	afterMatch, _ := s.GetTask(matchTask.ID)
	afterOther, _ := s.GetTask(otherTask.ID)
	if afterMatch.Priority != 3 {
		t.Fatalf("expected matched task priority 3, got %d", afterMatch.Priority)
	}
	if afterOther.Priority != 2 {
		t.Fatalf("expected unmatched task to remain 2, got %d", afterOther.Priority)
	}
}

func TestComputeOperationalMetrics(t *testing.T) {
	s := setupTestStore(t)

	// Notes: 3 total, 1 duplicate (same normalized content)
	if _, err := s.CreateNote("repeat me", nil, nil, "manual"); err != nil {
		t.Fatalf("create note 1: %v", err)
	}
	if _, err := s.CreateNote("  Repeat Me  ", nil, nil, "sync"); err != nil {
		t.Fatalf("create note 2: %v", err)
	}
	if _, err := s.CreateNote("unique note", nil, nil, "sync"); err != nil {
		t.Fatalf("create note 3: %v", err)
	}

	// Tasks: 2 total, 1 done.
	taskDone, _ := s.CreateTask("done", "", nil, 1)
	taskTodo, _ := s.CreateTask("todo", "", nil, 1)
	if err := s.UpdateTaskStatus(taskDone.ID, model.TaskDone); err != nil {
		t.Fatalf("set task done: %v", err)
	}
	if err := s.UpdateTaskStatus(taskTodo.ID, model.TaskTodo); err != nil {
		t.Fatalf("set task todo: %v", err)
	}

	// KB updates: a.md updated twice, b.md once.
	syncLog, _ := s.CreateSyncLog("agent", "prompt")
	if err := s.UpdateSyncLog(syncLog.ID, model.SyncCompleted, "", 0, 0, "a.md,b.md", 0, ""); err != nil {
		t.Fatalf("update sync log: %v", err)
	}
	conLog, _ := s.CreateSleepConsolidationLog("agent")
	if err := s.UpdateConsolidationLog(conLog.ID, model.ConsolidationCompleted, "", "a.md"); err != nil {
		t.Fatalf("update consolidation log: %v", err)
	}

	metrics, err := s.ComputeOperationalMetrics(14)
	if err != nil {
		t.Fatalf("compute metrics: %v", err)
	}

	if metrics.NotesTotal != 3 || metrics.DuplicateNotes != 1 {
		t.Fatalf("unexpected note metrics: %+v", metrics)
	}
	if !almostEqual(metrics.DuplicateNoteRate, 1.0/3.0) {
		t.Fatalf("unexpected duplicate note rate: %f", metrics.DuplicateNoteRate)
	}
	if metrics.TasksTotal != 2 || metrics.TasksDone != 1 {
		t.Fatalf("unexpected task metrics: %+v", metrics)
	}
	if !almostEqual(metrics.UsefulTaskGenerationRate, 0.5) {
		t.Fatalf("unexpected useful task rate: %f", metrics.UsefulTaskGenerationRate)
	}
	if metrics.UniqueKBFilesUpdated != 2 || metrics.ReworkedKBFiles != 1 {
		t.Fatalf("unexpected KB metrics: %+v", metrics)
	}
	if !almostEqual(metrics.KBReworkRate, 0.5) {
		t.Fatalf("unexpected kb rework rate: %f", metrics.KBReworkRate)
	}
}

func TestLinkNotesUpsertReinforcesWeight(t *testing.T) {
	s := setupTestStore(t)
	a, _ := s.CreateNote("A", nil, nil, "")
	b, _ := s.CreateNote("B", nil, nil, "")

	if err := s.LinkNotes(a.ID, b.ID, 0.40, "first"); err != nil {
		t.Fatalf("link notes first: %v", err)
	}
	var weight float64
	var reinforced int
	if err := s.db.QueryRow(
		`SELECT weight, reinforced_count FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&weight, &reinforced); err != nil {
		t.Fatalf("query edge: %v", err)
	}
	if !almostEqual(weight, 0.40) || reinforced != 1 {
		t.Fatalf("unexpected first edge state: weight=%f reinforced=%d", weight, reinforced)
	}

	if err := s.LinkNotes(a.ID, b.ID, 0.40, "second"); err != nil {
		t.Fatalf("link notes second: %v", err)
	}
	if err := s.db.QueryRow(
		`SELECT weight, reinforced_count FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&weight, &reinforced); err != nil {
		t.Fatalf("query reinforced edge: %v", err)
	}
	if weight <= 0.40 || weight > 1 {
		t.Fatalf("expected reinforced weight in (0.40,1], got %f", weight)
	}
	if reinforced != 2 {
		t.Fatalf("expected reinforced_count=2, got %d", reinforced)
	}
}

func TestCreateNoteAutoLinksSimilarNotesWhenEnabled(t *testing.T) {
	s := setupTestStore(t)

	first, _ := s.CreateNote("cache invalidation strategy for api gateway", nil, []string{"cache", "api"}, "manual")
	second, _ := s.CreateNote("api gateway cache ttl backoff strategy", nil, []string{"cache", "api"}, "manual")
	noise, _ := s.CreateNote("frontend typography and spacing baseline", nil, []string{"design"}, "manual")

	t.Setenv("SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK", "1")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS", "2")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE", "0.20")

	newNote, _ := s.CreateNote("api gateway cache invalidation and ttl strategy", nil, []string{"cache", "api"}, "manual")

	related, err := s.RelatedNotes(newNote.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes for new note: %v", err)
	}
	if !hasRelatedNoteID(related, first.ID) {
		t.Fatalf("expected auto-link to first note %d, got %+v", first.ID, related)
	}
	if !hasRelatedNoteID(related, second.ID) {
		t.Fatalf("expected auto-link to second note %d, got %+v", second.ID, related)
	}
	if hasRelatedNoteID(related, noise.ID) {
		t.Fatalf("did not expect unrelated note %d in auto-link results: %+v", noise.ID, related)
	}

	reverse, err := s.RelatedNotes(first.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes for reverse check: %v", err)
	}
	if !hasRelatedNoteID(reverse, newNote.ID) {
		t.Fatalf("expected reverse auto-link from %d to %d, got %+v", first.ID, newNote.ID, reverse)
	}
}

func TestCreateNoteAutoLinkDisabled(t *testing.T) {
	s := setupTestStore(t)

	first, _ := s.CreateNote("incident rollback checklist", nil, []string{"ops"}, "manual")
	second, _ := s.CreateNote("incident rollback checklist and owner rotation", nil, []string{"ops"}, "manual")

	related, err := s.RelatedNotes(second.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes: %v", err)
	}
	if hasRelatedNoteID(related, first.ID) {
		t.Fatalf("did not expect auto-link while feature disabled, got %+v", related)
	}
}

func TestCreateNoteAutoLinkRespectsMaxLinks(t *testing.T) {
	s := setupTestStore(t)

	c1, _ := s.CreateNote("query planner index strategy", nil, []string{"db"}, "manual")
	c2, _ := s.CreateNote("query planner join strategy", nil, []string{"db"}, "manual")
	c3, _ := s.CreateNote("query planner cache strategy", nil, []string{"db"}, "manual")

	t.Setenv("SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK", "1")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS", "1")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE", "0.10")

	newNote, _ := s.CreateNote("query planner strategy for index join cache", nil, []string{"db"}, "manual")

	related, err := s.RelatedNotes(newNote.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes: %v", err)
	}
	if len(related) != 1 {
		t.Fatalf("expected exactly 1 direct auto-link, got %d (%+v)", len(related), related)
	}

	target := related[0].Note.ID
	if target != c1.ID && target != c2.ID && target != c3.ID {
		t.Fatalf("unexpected auto-link target: %d", target)
	}
}

func TestLearnEntitiesFromNote(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote(
		"Discussed @grace_hopper and #compiler strategy",
		nil,
		[]string{"person:Grace Hopper", "concept:Compiler", "org:navy"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities from note: %v", err)
	}

	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities by note: %v", err)
	}
	if len(entities) < 3 {
		t.Fatalf("expected at least 3 learned entities, got %d (%+v)", len(entities), entities)
	}

	kinds := map[string]int{}
	for _, entity := range entities {
		kinds[entity.Kind]++
	}
	if kinds["person"] == 0 {
		t.Fatalf("expected at least one person entity, got %+v", kinds)
	}
	if kinds["concept"] == 0 {
		t.Fatalf("expected at least one concept entity, got %+v", kinds)
	}
	if kinds["org"] == 0 {
		t.Fatalf("expected at least one org entity, got %+v", kinds)
	}

	var edgeCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM entity_edges`).Scan(&edgeCount); err != nil {
		t.Fatalf("count entity_edges: %v", err)
	}
	if edgeCount == 0 {
		t.Fatal("expected entity cooccurrence edges to be created")
	}
}

func TestLearnEntitiesFromNoteRespectsFeatureFlag(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_ENTITY_LEARNING", "0")

	note, err := s.CreateNote("Person tag only", nil, []string{"person:Alan Turing"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities from note: %v", err)
	}

	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities by note: %v", err)
	}
	if len(entities) != 0 {
		t.Fatalf("expected no entities when feature disabled, got %+v", entities)
	}
}

func TestLearnEntitiesFromNotePromotesEntityAfterRepeatedEvidence(t *testing.T) {
	s := setupTestStore(t)

	first, err := s.CreateNote("Grace Hopper early memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create first note: %v", err)
	}
	second, err := s.CreateNote("Grace Hopper compiler note", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create second note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*first, "consolidation_apply"); err != nil {
		t.Fatalf("learn first note entities: %v", err)
	}
	firstEntities, err := s.ListEntitiesByNote(first.ID)
	if err != nil {
		t.Fatalf("list first entities: %v", err)
	}
	firstPerson, ok := findEntityByKindAndName(firstEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity in first note, got %+v", firstEntities)
	}
	if firstPerson.Status != "candidate" {
		t.Fatalf("expected candidate after single supporting note, got %s", firstPerson.Status)
	}

	if err := s.LearnEntitiesFromNote(*second, "consolidation_apply"); err != nil {
		t.Fatalf("learn second note entities: %v", err)
	}
	secondEntities, err := s.ListEntitiesByNote(second.ID)
	if err != nil {
		t.Fatalf("list second entities: %v", err)
	}
	secondPerson, ok := findEntityByKindAndName(secondEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity in second note, got %+v", secondEntities)
	}
	if secondPerson.Status != "confirmed" {
		t.Fatalf("expected confirmed after repeated supporting notes, got %s", secondPerson.Status)
	}

	firstEntitiesAgain, err := s.ListEntitiesByNote(first.ID)
	if err != nil {
		t.Fatalf("list first entities again: %v", err)
	}
	firstPersonAgain, ok := findEntityByKindAndName(firstEntitiesAgain, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity in first note after promotion, got %+v", firstEntitiesAgain)
	}
	if firstPersonAgain.Status != "confirmed" {
		t.Fatalf("expected shared entity status to propagate as confirmed, got %s", firstPersonAgain.Status)
	}
}

func TestListEntitiesSupportsKindAndStatusFilters(t *testing.T) {
	s := setupTestStore(t)

	first, err := s.CreateNote(
		"Grace Hopper compiler note",
		nil,
		[]string{"person:Grace Hopper", "concept:Compiler"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create first note: %v", err)
	}
	second, err := s.CreateNote(
		"Grace Hopper design memo",
		nil,
		[]string{"person:Grace Hopper", "concept:Design"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create second note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*first, "consolidation_apply"); err != nil {
		t.Fatalf("learn first note entities: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*second, "consolidation_apply"); err != nil {
		t.Fatalf("learn second note entities: %v", err)
	}

	personKind := "person"
	personOnly, err := s.ListEntities(EntityFilter{Kind: &personKind, Limit: 10})
	if err != nil {
		t.Fatalf("list entities by kind: %v", err)
	}
	if len(personOnly) == 0 {
		t.Fatal("expected person entities")
	}
	for _, entity := range personOnly {
		if entity.Kind != "person" {
			t.Fatalf("expected only person kind, got %+v", entity)
		}
	}

	confirmed := "confirmed"
	confirmedOnly, err := s.ListEntities(EntityFilter{Status: &confirmed, Limit: 10})
	if err != nil {
		t.Fatalf("list entities by status: %v", err)
	}
	if len(confirmedOnly) == 0 {
		t.Fatal("expected at least one confirmed entity")
	}
	for _, entity := range confirmedOnly {
		if entity.Status != "confirmed" {
			t.Fatalf("expected only confirmed status, got %+v", entity)
		}
	}

	got, err := s.GetEntity(confirmedOnly[0].ID)
	if err != nil {
		t.Fatalf("get entity by id: %v", err)
	}
	if got.ID != confirmedOnly[0].ID {
		t.Fatalf("expected same entity id, got %d want %d", got.ID, confirmedOnly[0].ID)
	}
}

func TestUpdateEntityStatus(t *testing.T) {
	s := setupTestStore(t)

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}
	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected learned entities")
	}
	entityID := entities[0].ID

	if err := s.UpdateEntityStatus(entityID, "rejected"); err != nil {
		t.Fatalf("update entity status: %v", err)
	}
	updated, err := s.GetEntity(entityID)
	if err != nil {
		t.Fatalf("get entity after update: %v", err)
	}
	if updated.Status != "rejected" {
		t.Fatalf("expected entity status rejected, got %s", updated.Status)
	}
}

func TestUpdateEntityStatusValidation(t *testing.T) {
	s := setupTestStore(t)

	if err := s.UpdateEntityStatus(1, "invalid-status"); err == nil {
		t.Fatal("expected invalid status error")
	}
	if err := s.UpdateEntityStatus(9999, "confirmed"); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestLearnEntitiesFromNoteCreatesEntityDerivedMemoryEdges(t *testing.T) {
	s := setupTestStore(t)

	anchor, err := s.CreateNote(
		"Grace Hopper compiler retrospective",
		nil,
		[]string{"person:Grace Hopper", "concept:Compiler"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create anchor note: %v", err)
	}
	unrelated, err := s.CreateNote(
		"Typography and spacing guideline",
		nil,
		[]string{"concept:Design"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create unrelated note: %v", err)
	}
	target, err := s.CreateNote(
		"Grace Hopper COBOL compiler memo",
		nil,
		[]string{"person:Grace Hopper", "concept:COBOL"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create target note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*anchor, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for anchor: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*unrelated, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for unrelated: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*target, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for target: %v", err)
	}

	var (
		weight   float64
		evidence string
	)
	if err := s.db.QueryRow(
		`SELECT weight, evidence FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		target.ID, anchor.ID,
	).Scan(&weight, &evidence); err != nil {
		t.Fatalf("query entity-derived edge target->anchor: %v", err)
	}
	if weight <= 0 {
		t.Fatalf("expected positive entity-derived edge weight, got %f", weight)
	}
	if !strings.HasPrefix(evidence, "auto:entity-shared") {
		t.Fatalf("expected entity-derived edge evidence prefix, got %q", evidence)
	}
	if err := s.db.QueryRow(
		`SELECT weight, evidence FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		anchor.ID, target.ID,
	).Scan(&weight, &evidence); err != nil {
		t.Fatalf("query entity-derived edge anchor->target: %v", err)
	}
	if weight <= 0 {
		t.Fatalf("expected positive reverse entity-derived edge weight, got %f", weight)
	}

	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		target.ID, unrelated.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count target->unrelated edge: %v", err)
	}
	if count != 0 {
		t.Fatalf("did not expect unrelated entity-derived edge, count=%d", count)
	}
}

func TestLearnEntitiesFromNoteEntityDerivedEdgesRespectMinShared(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MIN_SHARED", "2")

	anchor, err := s.CreateNote(
		"Grace Hopper compiler notes",
		nil,
		[]string{"person:Grace Hopper", "concept:Compiler"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create anchor note: %v", err)
	}
	target, err := s.CreateNote(
		"Grace Hopper onboarding memo",
		nil,
		[]string{"person:Grace Hopper"},
		"manual",
	)
	if err != nil {
		t.Fatalf("create target note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*anchor, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for anchor: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*target, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for target: %v", err)
	}

	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		target.ID, anchor.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count target->anchor edge: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no entity-derived edge with shared entity below threshold, count=%d", count)
	}
}

func TestLearnEntitiesFromNoteEntityDerivedEdgesRespectFeatureFlag(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_ENTITY_DERIVED_EDGE", "0")

	anchor, err := s.CreateNote("Grace Hopper compiler", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create anchor note: %v", err)
	}
	target, err := s.CreateNote("Grace Hopper memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create target note: %v", err)
	}

	if err := s.LearnEntitiesFromNote(*anchor, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for anchor: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*target, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities for target: %v", err)
	}

	var edgeCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_edges`).Scan(&edgeCount); err != nil {
		t.Fatalf("count memory edges: %v", err)
	}
	if edgeCount != 0 {
		t.Fatalf("expected no memory edge when entity-derived edge feature disabled, got %d", edgeCount)
	}
}

func TestDecayEntities(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_DECAY_RATE", "1.0")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.10")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.20")

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}

	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before decay: %v", err)
	}
	person, ok := findEntityByKindAndName(entities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity before decay, got %+v", entities)
	}

	base := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE entities SET strength = ?, salience = ?, status = 'confirmed', updated_at = ? WHERE id = ?`,
		0.90, 0.90, base, person.ID,
	); err != nil {
		t.Fatalf("seed entity state: %v", err)
	}

	affected, err := s.DecayEntities(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay entities: %v", err)
	}
	if affected < 1 {
		t.Fatalf("expected at least 1 decayed entity, got %d", affected)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after decay: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity after decay, got %+v", afterEntities)
	}
	if afterPerson.Strength >= 0.90 {
		t.Fatalf("expected decayed entity strength, got %f", afterPerson.Strength)
	}
	if afterPerson.Salience >= 0.90 {
		t.Fatalf("expected decayed entity salience, got %f", afterPerson.Salience)
	}
	if afterPerson.Status != "candidate" {
		t.Fatalf("expected confirmed entity to demote to candidate, got %s", afterPerson.Status)
	}
}

func TestDecayEntitiesRespectsMinimums(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_DECAY_RATE", "1.0")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.25")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.35")

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}
	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before decay: %v", err)
	}
	person, ok := findEntityByKindAndName(entities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity before decay, got %+v", entities)
	}

	base := time.Now().UTC().Add(-240 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE entities SET strength = ?, salience = ?, updated_at = ? WHERE id = ?`,
		0.30, 0.40, base, person.ID,
	); err != nil {
		t.Fatalf("seed entity state: %v", err)
	}

	if _, err := s.DecayEntities(time.Now().UTC()); err != nil {
		t.Fatalf("decay entities: %v", err)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after decay: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity after decay, got %+v", afterEntities)
	}
	if afterPerson.Strength < 0.25 {
		t.Fatalf("expected strength floor 0.25, got %f", afterPerson.Strength)
	}
	if afterPerson.Salience < 0.35 {
		t.Fatalf("expected salience floor 0.35, got %f", afterPerson.Salience)
	}
}

func TestDecayEntitiesDisabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_ENTITY_DECAY", "0")

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}
	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before decay: %v", err)
	}
	person, ok := findEntityByKindAndName(entities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity before decay, got %+v", entities)
	}

	base := time.Now().UTC().Add(-240 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE entities SET strength = ?, salience = ?, updated_at = ? WHERE id = ?`,
		0.80, 0.85, base, person.ID,
	); err != nil {
		t.Fatalf("seed entity state: %v", err)
	}

	affected, err := s.DecayEntities(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay entities: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected 0 affected entities when decay disabled, got %d", affected)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after decay: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity after decay, got %+v", afterEntities)
	}
	if !almostEqual(afterPerson.Strength, 0.80) {
		t.Fatalf("expected unchanged strength when decay disabled, got %f", afterPerson.Strength)
	}
	if !almostEqual(afterPerson.Salience, 0.85) {
		t.Fatalf("expected unchanged salience when decay disabled, got %f", afterPerson.Salience)
	}
}

func TestDecayEntitiesDoesNotIncreaseWeakValues(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_ENTITY_DECAY_RATE", "1.0")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.10")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.20")

	note, err := s.CreateNote("Grace Hopper compiler memo", nil, []string{"person:Grace Hopper"}, "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.LearnEntitiesFromNote(*note, "consolidation_apply"); err != nil {
		t.Fatalf("learn entities: %v", err)
	}
	entities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities before decay: %v", err)
	}
	person, ok := findEntityByKindAndName(entities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity before decay, got %+v", entities)
	}

	base := time.Now().UTC().Add(-240 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE entities SET strength = ?, salience = ?, updated_at = ? WHERE id = ?`,
		0.08, 0.18, base, person.ID,
	); err != nil {
		t.Fatalf("seed weak entity state: %v", err)
	}

	affected, err := s.DecayEntities(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay entities: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected no affected entity when values are already below floor, got %d", affected)
	}

	afterEntities, err := s.ListEntitiesByNote(note.ID)
	if err != nil {
		t.Fatalf("list entities after decay: %v", err)
	}
	afterPerson, ok := findEntityByKindAndName(afterEntities, "person", "grace hopper")
	if !ok {
		t.Fatalf("expected person entity after decay, got %+v", afterEntities)
	}
	if !almostEqual(afterPerson.Strength, 0.08) {
		t.Fatalf("expected no strength increase during decay, got %f", afterPerson.Strength)
	}
	if !almostEqual(afterPerson.Salience, 0.18) {
		t.Fatalf("expected no salience increase during decay, got %f", afterPerson.Salience)
	}
}

func TestRelatedNotes(t *testing.T) {
	s := setupTestStore(t)
	a, _ := s.CreateNote("Seed", nil, nil, "")
	b, _ := s.CreateNote("B direct", nil, nil, "")
	c, _ := s.CreateNote("C direct", nil, nil, "")
	d, _ := s.CreateNote("D indirect", nil, nil, "")

	if err := s.LinkNotes(a.ID, b.ID, 0.90, "a->b"); err != nil {
		t.Fatalf("link a->b: %v", err)
	}
	if err := s.LinkNotes(a.ID, c.ID, 0.30, "a->c"); err != nil {
		t.Fatalf("link a->c: %v", err)
	}
	if err := s.LinkNotes(b.ID, d.ID, 0.80, "b->d"); err != nil {
		t.Fatalf("link b->d: %v", err)
	}

	depth1, err := s.RelatedNotes(a.ID, 1, 10)
	if err != nil {
		t.Fatalf("related notes depth1: %v", err)
	}
	if len(depth1) != 2 {
		t.Fatalf("expected 2 direct related notes, got %d", len(depth1))
	}
	if depth1[0].Note.ID != b.ID || depth1[1].Note.ID != c.ID {
		t.Fatalf("unexpected depth1 order: [%d, %d]", depth1[0].Note.ID, depth1[1].Note.ID)
	}

	depth2, err := s.RelatedNotes(a.ID, 2, 10)
	if err != nil {
		t.Fatalf("related notes depth2: %v", err)
	}
	foundIndirect := false
	for _, rn := range depth2 {
		if rn.Note.ID == d.ID {
			foundIndirect = true
			break
		}
	}
	if !foundIndirect {
		t.Fatal("expected indirect note to appear with depth=2")
	}
}

func TestRelatedNotesAvoidsCycleAmplification(t *testing.T) {
	s := setupTestStore(t)
	a, _ := s.CreateNote("A", nil, nil, "")
	b, _ := s.CreateNote("B", nil, nil, "")
	c, _ := s.CreateNote("C", nil, nil, "")

	if err := s.LinkNotes(a.ID, b.ID, 0.9, "a->b"); err != nil {
		t.Fatalf("link a->b: %v", err)
	}
	if err := s.LinkNotes(b.ID, a.ID, 0.9, "b->a"); err != nil {
		t.Fatalf("link b->a: %v", err)
	}
	if err := s.LinkNotes(b.ID, c.ID, 0.5, "b->c"); err != nil {
		t.Fatalf("link b->c: %v", err)
	}

	related, err := s.RelatedNotes(a.ID, 3, 10)
	if err != nil {
		t.Fatalf("related notes depth3: %v", err)
	}

	scoreByID := map[int64]float64{}
	for _, rn := range related {
		scoreByID[rn.Note.ID] = rn.Score
	}

	if !almostEqual(scoreByID[b.ID], 0.9) {
		t.Fatalf("expected B score to remain direct-only 0.9, got %f", scoreByID[b.ID])
	}
	if scoreByID[c.ID] <= 0 {
		t.Fatal("expected C to appear through B with positive score")
	}
}

func TestDecayMemoryEdges(t *testing.T) {
	s := setupTestStore(t)

	a, _ := s.CreateNote("edge source", nil, nil, "manual")
	b, _ := s.CreateNote("edge target", nil, nil, "manual")
	if err := s.LinkNotes(a.ID, b.ID, 0.80, "decay-target"); err != nil {
		t.Fatalf("link notes: %v", err)
	}

	past := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE memory_edges SET updated_at = ? WHERE from_note_id = ? AND to_note_id = ?`,
		past, a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed edge timestamp: %v", err)
	}

	var before float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&before); err != nil {
		t.Fatalf("query edge before decay: %v", err)
	}

	affected, err := s.DecayMemoryEdges(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay memory edges: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 decayed edge, got %d", affected)
	}

	var after float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&after); err != nil {
		t.Fatalf("query edge after decay: %v", err)
	}
	if after >= before {
		t.Fatalf("expected edge weight to decrease: before=%f after=%f", before, after)
	}
}

func TestDecayMemoryEdgesRespectsMinWeight(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "1.0")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "0.25")

	a, _ := s.CreateNote("edge source", nil, nil, "manual")
	b, _ := s.CreateNote("edge target", nil, nil, "manual")
	if err := s.LinkNotes(a.ID, b.ID, 0.30, "decay-floor"); err != nil {
		t.Fatalf("link notes: %v", err)
	}

	past := time.Now().UTC().Add(-365 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE memory_edges SET updated_at = ? WHERE from_note_id = ? AND to_note_id = ?`,
		past, a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed edge timestamp: %v", err)
	}

	_, err := s.DecayMemoryEdges(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay memory edges: %v", err)
	}

	var weight float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&weight); err != nil {
		t.Fatalf("query decayed edge: %v", err)
	}
	if weight < 0.25 {
		t.Fatalf("expected decayed weight to respect minimum 0.25, got %f", weight)
	}
}

func TestDecayMemoryEdgesDisabled(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_FEATURE_MEMORY_EDGE_DECAY", "0")

	a, _ := s.CreateNote("edge source", nil, nil, "manual")
	b, _ := s.CreateNote("edge target", nil, nil, "manual")
	if err := s.LinkNotes(a.ID, b.ID, 0.70, "decay-disabled"); err != nil {
		t.Fatalf("link notes: %v", err)
	}

	past := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE memory_edges SET updated_at = ? WHERE from_note_id = ? AND to_note_id = ?`,
		past, a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed edge timestamp: %v", err)
	}

	var before float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&before); err != nil {
		t.Fatalf("query edge before disabled decay: %v", err)
	}

	affected, err := s.DecayMemoryEdges(time.Now().UTC())
	if err != nil {
		t.Fatalf("decay memory edges: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected 0 decayed edges when disabled, got %d", affected)
	}

	var after float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&after); err != nil {
		t.Fatalf("query edge after disabled decay: %v", err)
	}
	if !almostEqual(before, after) {
		t.Fatalf("expected unchanged edge weight when decay disabled: before=%f after=%f", before, after)
	}
}

func TestDecayMemoryEdgesKeepsPositiveWeightWhenMinIsZero(t *testing.T) {
	s := setupTestStore(t)
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "1.0")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "0")

	a, _ := s.CreateNote("edge source", nil, nil, "manual")
	b, _ := s.CreateNote("edge target", nil, nil, "manual")
	if err := s.LinkNotes(a.ID, b.ID, 0.20, "decay-zero-floor"); err != nil {
		t.Fatalf("link notes: %v", err)
	}

	past := time.Now().UTC().Add(-2000 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE memory_edges SET updated_at = ? WHERE from_note_id = ? AND to_note_id = ?`,
		past, a.ID, b.ID,
	); err != nil {
		t.Fatalf("seed edge timestamp: %v", err)
	}

	if _, err := s.DecayMemoryEdges(time.Now().UTC()); err != nil {
		t.Fatalf("decay memory edges with zero floor: %v", err)
	}

	var weight float64
	if err := s.db.QueryRow(
		`SELECT weight FROM memory_edges WHERE from_note_id = ? AND to_note_id = ?`,
		a.ID, b.ID,
	).Scan(&weight); err != nil {
		t.Fatalf("query decayed edge: %v", err)
	}
	if weight <= 0 {
		t.Fatalf("expected strictly positive edge weight, got %f", weight)
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

func tableExists(db *sql.DB, tableName string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func indexExists(db *sql.DB, indexName string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`,
		indexName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

func hasRelatedNoteID(related []model.RelatedNote, noteID int64) bool {
	for _, rn := range related {
		if rn.Note.ID == noteID {
			return true
		}
	}
	return false
}

func findEntityByKindAndName(entities []model.Entity, kind string, needle string) (model.Entity, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, entity := range entities {
		if strings.ToLower(entity.Kind) != kind {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(entity.CanonicalName), needle) {
			continue
		}
		return entity, true
	}
	return model.Entity{}, false
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
