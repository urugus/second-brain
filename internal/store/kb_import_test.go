package store

import (
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func TestImportKBNote(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	meta := model.KBMetadata{
		Strength:       0.72,
		Salience:       0.65,
		DecayRate:      0.012,
		RecallCount:    5,
		Source:         "consolidation",
		Tags:           []string{"golang", "testing"},
		ConsolidatedAt: &now,
	}

	id, err := s.ImportKBNote("# Test Content\nBody here.", meta)
	if err != nil {
		t.Fatalf("ImportKBNote: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive note ID")
	}

	// Verify the note was created with front matter weights
	note, err := s.GetNote(id)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}

	if note.Strength != 0.72 {
		t.Errorf("strength: got %f, want 0.72", note.Strength)
	}
	if note.Salience != 0.65 {
		t.Errorf("salience: got %f, want 0.65", note.Salience)
	}
	if note.DecayRate != 0.012 {
		t.Errorf("decay_rate: got %f, want 0.012", note.DecayRate)
	}
	if note.RecallCount != 5 {
		t.Errorf("recall_count: got %d, want 5", note.RecallCount)
	}
	if note.Source != "consolidation" {
		t.Errorf("source: got %q, want consolidation", note.Source)
	}
	if note.ConsolidatedAt == nil {
		t.Error("consolidated_at should not be nil")
	}
}

func TestImportKBNoteDefaults(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	meta := model.KBMetadata{} // all zero values

	id, err := s.ImportKBNote("Content", meta)
	if err != nil {
		t.Fatalf("ImportKBNote: %v", err)
	}

	note, err := s.GetNote(id)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}

	if note.Strength != 0.30 {
		t.Errorf("default strength: got %f, want 0.30", note.Strength)
	}
	if note.Salience != 0.50 {
		t.Errorf("default salience: got %f, want 0.50", note.Salience)
	}
	if note.DecayRate != defaultDecayRate {
		t.Errorf("default decay_rate: got %f, want %f", note.DecayRate, defaultDecayRate)
	}
	if note.Source != "kb-import" {
		t.Errorf("default source: got %q, want kb-import", note.Source)
	}
}

func TestImportKBEdges(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	meta1 := model.KBMetadata{Strength: 0.5, Salience: 0.5, DecayRate: 0.015}
	meta2 := model.KBMetadata{Strength: 0.6, Salience: 0.6, DecayRate: 0.015}

	id1, _ := s.ImportKBNote("Note 1", meta1)
	id2, _ := s.ImportKBNote("Note 2", meta2)

	kbPathToNoteID := map[string]int64{
		"kb/file1.md": id1,
		"kb/file2.md": id2,
	}

	related := []model.KBRelatedEntry{
		{Path: "kb/file2.md", Weight: 0.45},
	}

	err := s.ImportKBEdges(id1, related, kbPathToNoteID)
	if err != nil {
		t.Fatalf("ImportKBEdges: %v", err)
	}

	// Verify edge was created by checking related notes
	relatedNotes, err := s.RelatedNotes(id1, 1, 5)
	if err != nil {
		t.Fatalf("RelatedNotes: %v", err)
	}
	if len(relatedNotes) == 0 {
		t.Fatal("expected at least 1 related note")
	}
}

func TestImportKBNoteAndMapKB(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	meta := model.KBMetadata{
		Strength:  0.72,
		Salience:  0.65,
		DecayRate: 0.015,
	}

	id, err := s.ImportKBNote("Content", meta)
	if err != nil {
		t.Fatalf("ImportKBNote: %v", err)
	}

	// Map the note to a KB path
	if err := s.MapKBNotes("testing/approach.md", []int64{id}); err != nil {
		t.Fatalf("MapKBNotes: %v", err)
	}

	// Verify NotesByKBPath returns the note
	notes, err := s.NotesByKBPath("testing/approach.md")
	if err != nil {
		t.Fatalf("NotesByKBPath: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].ID != id {
		t.Errorf("note ID: got %d, want %d", notes[0].ID, id)
	}
	if notes[0].Strength != 0.72 {
		t.Errorf("note strength: got %f, want 0.72", notes[0].Strength)
	}
}
