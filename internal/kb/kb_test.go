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

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	err := k.Write("test.md", "# Test\nContent here.\n")
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	content, err := k.Read("test.md")
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if content != "# Test\nContent here.\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestWriteNested(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	err := k.Write(filepath.Join("sub", "topic.md"), "# Topic\n")
	if err != nil {
		t.Fatalf("write nested: %v", err)
	}

	if !k.Exists(filepath.Join("sub", "topic.md")) {
		t.Fatal("expected file to exist")
	}
}

func TestExtractTitle(t *testing.T) {
	k := testKB(t)

	title, err := k.ExtractTitle("golang-tips.md")
	if err != nil {
		t.Fatalf("extract title: %v", err)
	}
	if title != "Go Tips" {
		t.Fatalf("expected 'Go Tips', got %q", title)
	}
}

func TestExtractTitleNested(t *testing.T) {
	k := testKB(t)

	title, err := k.ExtractTitle(filepath.Join("nested", "architecture.md"))
	if err != nil {
		t.Fatalf("extract title: %v", err)
	}
	if title != "Architecture Notes" {
		t.Fatalf("expected 'Architecture Notes', got %q", title)
	}
}

func TestExtractTitleNoHeading(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	k.Write("no-heading.md", "Just plain text without heading.\n")

	title, err := k.ExtractTitle("no-heading.md")
	if err != nil {
		t.Fatalf("extract title: %v", err)
	}
	if title != "no-heading" {
		t.Fatalf("expected fallback 'no-heading', got %q", title)
	}
}

func TestWriteDirectoryTraversal(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	err := k.Write("../../evil.md", "bad content")
	if err == nil {
		t.Fatal("expected error for directory traversal in write")
	}
}

func TestExists(t *testing.T) {
	k := testKB(t)

	if !k.Exists("golang-tips.md") {
		t.Fatal("expected file to exist")
	}
	if k.Exists("nonexistent.md") {
		t.Fatal("expected file to not exist")
	}
}

func TestWriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	k.Write("doc.md", "v1")
	k.Write("doc.md", "v2")

	content, _ := k.Read("doc.md")
	if content != "v2" {
		t.Fatalf("expected v2, got %q", content)
	}
}
