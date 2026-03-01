package kb

import (
	"os"
	"path/filepath"
	"testing"
)

func testKB(t *testing.T) *KB {
	t.Helper()
	// Find testdata relative to this test file
	dir, _ := os.Getwd()
	root := filepath.Join(dir, "..", "..", "testdata", "knowledge")
	return New(root)
}

func TestList(t *testing.T) {
	k := testKB(t)
	files, err := k.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(files))
	}

	// Check that paths are relative
	for _, f := range files {
		if filepath.IsAbs(f) {
			t.Errorf("expected relative path, got %q", f)
		}
	}
}

func TestRead(t *testing.T) {
	k := testKB(t)

	content, err := k.Read("golang-tips.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty content")
	}
	if len(content) < 10 {
		t.Fatal("content too short")
	}
}

func TestReadNested(t *testing.T) {
	k := testKB(t)

	content, err := k.Read(filepath.Join("nested", "architecture.md"))
	if err != nil {
		t.Fatalf("read nested: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty content")
	}
}

func TestReadDirectoryTraversal(t *testing.T) {
	k := testKB(t)

	_, err := k.Read("../../go.mod")
	if err == nil {
		t.Fatal("expected error for directory traversal")
	}
}

func TestSearch(t *testing.T) {
	k := testKB(t)

	results, err := k.Search("adapter")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// Case insensitive
	results2, err := k.Search("ADAPTER")
	if err != nil {
		t.Fatalf("search case: %v", err)
	}
	if len(results2) != len(results) {
		t.Fatalf("case insensitive search: expected %d results, got %d", len(results), len(results2))
	}
}

func TestSearchNoResults(t *testing.T) {
	k := testKB(t)

	results, err := k.Search("xyznonexistent")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestReadNonexistentFile(t *testing.T) {
	k := testKB(t)

	_, err := k.Read("nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
