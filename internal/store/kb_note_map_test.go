package store

import (
	"testing"
)

func TestMapKBNotes(t *testing.T) {
	s := setupTestStore(t)

	a, _ := s.CreateNote("note A", nil, nil, "")
	b, _ := s.CreateNote("note B", nil, nil, "")

	if err := s.MapKBNotes("golang/tips.md", []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("map kb notes: %v", err)
	}

	// Upsert should not fail
	if err := s.MapKBNotes("golang/tips.md", []int64{a.ID}); err != nil {
		t.Fatalf("map kb notes upsert: %v", err)
	}
}

func TestMapKBNotesEmpty(t *testing.T) {
	s := setupTestStore(t)

	if err := s.MapKBNotes("empty.md", nil); err != nil {
		t.Fatalf("expected no error for empty note IDs: %v", err)
	}
}

func TestRelatedKBFiles(t *testing.T) {
	s := setupTestStore(t)

	// Create notes
	a, _ := s.CreateNote("note A", nil, nil, "")
	b, _ := s.CreateNote("note B", nil, nil, "")
	c, _ := s.CreateNote("note C", nil, nil, "")

	// Create memory edges: A -> B (0.8), A -> C (0.3)
	if err := s.LinkNotes(a.ID, b.ID, 0.8, "a-b"); err != nil {
		t.Fatalf("link a->b: %v", err)
	}
	if err := s.LinkNotes(a.ID, c.ID, 0.3, "a-c"); err != nil {
		t.Fatalf("link a->c: %v", err)
	}

	// Map notes to KB files
	// File X has note A
	if err := s.MapKBNotes("topic/x.md", []int64{a.ID}); err != nil {
		t.Fatalf("map x: %v", err)
	}
	// File Y has note B
	if err := s.MapKBNotes("topic/y.md", []int64{b.ID}); err != nil {
		t.Fatalf("map y: %v", err)
	}
	// File Z has note C
	if err := s.MapKBNotes("topic/z.md", []int64{c.ID}); err != nil {
		t.Fatalf("map z: %v", err)
	}

	// Related to X should find Y (via A->B edge) and Z (via A->C edge)
	related, err := s.RelatedKBFiles("topic/x.md", 5)
	if err != nil {
		t.Fatalf("related kb files: %v", err)
	}

	if len(related) != 2 {
		t.Fatalf("expected 2 related KB files, got %d", len(related))
	}

	// Y should rank higher (weight 0.8 > 0.3)
	if related[0].Path != "topic/y.md" {
		t.Fatalf("expected y.md first, got %s", related[0].Path)
	}
	if related[1].Path != "topic/z.md" {
		t.Fatalf("expected z.md second, got %s", related[1].Path)
	}
}

func TestRelatedKBFilesExcludesSelf(t *testing.T) {
	s := setupTestStore(t)

	a, _ := s.CreateNote("note A", nil, nil, "")
	b, _ := s.CreateNote("note B", nil, nil, "")

	if err := s.LinkNotes(a.ID, b.ID, 0.9, "edge"); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Both notes mapped to same file
	if err := s.MapKBNotes("same.md", []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("map: %v", err)
	}

	related, err := s.RelatedKBFiles("same.md", 5)
	if err != nil {
		t.Fatalf("related: %v", err)
	}

	if len(related) != 0 {
		t.Fatalf("expected 0 related (self excluded), got %d", len(related))
	}
}

func TestRelatedKBFilesNoEdges(t *testing.T) {
	s := setupTestStore(t)

	a, _ := s.CreateNote("isolated", nil, nil, "")
	if err := s.MapKBNotes("alone.md", []int64{a.ID}); err != nil {
		t.Fatalf("map: %v", err)
	}

	related, err := s.RelatedKBFiles("alone.md", 5)
	if err != nil {
		t.Fatalf("related: %v", err)
	}

	if len(related) != 0 {
		t.Fatalf("expected 0 related, got %d", len(related))
	}
}

func TestRelatedKBFilesBidirectional(t *testing.T) {
	s := setupTestStore(t)

	a, _ := s.CreateNote("note A", nil, nil, "")
	b, _ := s.CreateNote("note B", nil, nil, "")

	// Edge only from B -> A (reverse direction)
	if err := s.LinkNotes(b.ID, a.ID, 0.7, "b->a"); err != nil {
		t.Fatalf("link: %v", err)
	}

	if err := s.MapKBNotes("file-a.md", []int64{a.ID}); err != nil {
		t.Fatalf("map a: %v", err)
	}
	if err := s.MapKBNotes("file-b.md", []int64{b.ID}); err != nil {
		t.Fatalf("map b: %v", err)
	}

	// file-a should find file-b even though edge is B->A (reverse)
	related, err := s.RelatedKBFiles("file-a.md", 5)
	if err != nil {
		t.Fatalf("related: %v", err)
	}

	if len(related) != 1 || related[0].Path != "file-b.md" {
		t.Fatalf("expected file-b.md via reverse edge, got %v", related)
	}
}
