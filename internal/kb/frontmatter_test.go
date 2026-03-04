package kb

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

func TestParseFrontMatterBasic(t *testing.T) {
	input := `---
strength: 0.72
salience: 0.65
decay_rate: 0.015
recall_count: 5
source: consolidation
---

# Testing Approach

Use table-driven tests.
`

	meta, body := ParseFrontMatter(input)

	if meta.Strength != 0.72 {
		t.Errorf("strength: got %f, want 0.72", meta.Strength)
	}
	if meta.Salience != 0.65 {
		t.Errorf("salience: got %f, want 0.65", meta.Salience)
	}
	if meta.DecayRate != 0.015 {
		t.Errorf("decay_rate: got %f, want 0.015", meta.DecayRate)
	}
	if meta.RecallCount != 5 {
		t.Errorf("recall_count: got %d, want 5", meta.RecallCount)
	}
	if meta.Source != "consolidation" {
		t.Errorf("source: got %q, want consolidation", meta.Source)
	}
	if !strings.Contains(body, "# Testing Approach") {
		t.Errorf("body should contain heading, got: %q", body)
	}
}

func TestParseFrontMatterWithTags(t *testing.T) {
	input := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
tags:
  - golang
  - testing
  - ci
---

# Content
`

	meta, _ := ParseFrontMatter(input)

	if len(meta.Tags) != 3 {
		t.Fatalf("tags: got %d, want 3", len(meta.Tags))
	}
	if meta.Tags[0] != "golang" || meta.Tags[1] != "testing" || meta.Tags[2] != "ci" {
		t.Errorf("tags: got %v", meta.Tags)
	}
}

func TestParseFrontMatterWithRelated(t *testing.T) {
	input := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
related:
  - path: architecture/patterns.md
    weight: 0.45
  - path: golang/tips.md
    weight: 0.32
---

# Content
`

	meta, _ := ParseFrontMatter(input)

	if len(meta.Related) != 2 {
		t.Fatalf("related: got %d, want 2", len(meta.Related))
	}
	if meta.Related[0].Path != "architecture/patterns.md" {
		t.Errorf("related[0].path: got %q", meta.Related[0].Path)
	}
	if meta.Related[0].Weight != 0.45 {
		t.Errorf("related[0].weight: got %f, want 0.45", meta.Related[0].Weight)
	}
	if meta.Related[1].Path != "golang/tips.md" {
		t.Errorf("related[1].path: got %q", meta.Related[1].Path)
	}
}

func TestParseFrontMatterWithConsolidatedAt(t *testing.T) {
	input := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
consolidated_at: 2026-03-01T10:00:00Z
---

# Content
`

	meta, _ := ParseFrontMatter(input)

	if meta.ConsolidatedAt == nil {
		t.Fatal("consolidated_at should not be nil")
	}
	expected, _ := time.Parse(time.RFC3339, "2026-03-01T10:00:00Z")
	if !meta.ConsolidatedAt.Equal(expected) {
		t.Errorf("consolidated_at: got %v, want %v", meta.ConsolidatedAt, expected)
	}
}

func TestParseFrontMatterNoFrontMatter(t *testing.T) {
	input := "# Just a heading\n\nSome content.\n"

	meta, body := ParseFrontMatter(input)

	if meta.Strength != 0 {
		t.Errorf("expected zero metadata, got strength=%f", meta.Strength)
	}
	if body != input {
		t.Errorf("body should be unchanged, got: %q", body)
	}
}

func TestParseFrontMatterInlineTagsList(t *testing.T) {
	input := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
tags: [golang, testing]
---

# Content
`

	meta, _ := ParseFrontMatter(input)

	if len(meta.Tags) != 2 {
		t.Fatalf("tags: got %d, want 2", len(meta.Tags))
	}
	if meta.Tags[0] != "golang" || meta.Tags[1] != "testing" {
		t.Errorf("tags: got %v", meta.Tags)
	}
}

func TestMarshalFrontMatter(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	meta := model.KBMetadata{
		Strength:       0.72,
		Salience:       0.65,
		DecayRate:      0.015,
		RecallCount:    5,
		Source:         "consolidation",
		Tags:           []string{"golang", "testing"},
		ConsolidatedAt: &now,
		Related: []model.KBRelatedEntry{
			{Path: "arch/patterns.md", Weight: 0.45},
		},
	}

	body := "# Testing Approach\n\nUse table-driven tests.\n"
	result := MarshalFrontMatter(meta, body)

	if !strings.HasPrefix(result, "---\n") {
		t.Error("should start with ---")
	}
	if !strings.Contains(result, "strength: 0.72") {
		t.Error("should contain strength")
	}
	if !strings.Contains(result, "salience: 0.65") {
		t.Error("should contain salience")
	}
	if !strings.Contains(result, "recall_count: 5") {
		t.Error("should contain recall_count")
	}
	if !strings.Contains(result, "source: consolidation") {
		t.Error("should contain source")
	}
	if !strings.Contains(result, "  - golang") {
		t.Error("should contain tags")
	}
	if !strings.Contains(result, "  - path: arch/patterns.md") {
		t.Error("should contain related path")
	}
	if !strings.Contains(result, "    weight: 0.45") {
		t.Error("should contain related weight")
	}
	if !strings.Contains(result, "# Testing Approach") {
		t.Error("should contain body")
	}
}

func TestMarshalThenParse(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	original := model.KBMetadata{
		Strength:       0.72,
		Salience:       0.65,
		DecayRate:      0.015,
		RecallCount:    5,
		Source:         "consolidation",
		Tags:           []string{"golang", "testing"},
		ConsolidatedAt: &now,
		Related: []model.KBRelatedEntry{
			{Path: "arch/patterns.md", Weight: 0.45},
			{Path: "golang/tips.md", Weight: 0.32},
		},
	}
	body := "# Testing\n\nContent here.\n"

	content := MarshalFrontMatter(original, body)
	parsed, parsedBody := ParseFrontMatter(content)

	if parsed.Strength != original.Strength {
		t.Errorf("strength roundtrip: got %f, want %f", parsed.Strength, original.Strength)
	}
	if parsed.Salience != original.Salience {
		t.Errorf("salience roundtrip: got %f, want %f", parsed.Salience, original.Salience)
	}
	if parsed.DecayRate != original.DecayRate {
		t.Errorf("decay_rate roundtrip: got %f, want %f", parsed.DecayRate, original.DecayRate)
	}
	if parsed.RecallCount != original.RecallCount {
		t.Errorf("recall_count roundtrip: got %d, want %d", parsed.RecallCount, original.RecallCount)
	}
	if parsed.Source != original.Source {
		t.Errorf("source roundtrip: got %q, want %q", parsed.Source, original.Source)
	}
	if len(parsed.Tags) != len(original.Tags) {
		t.Errorf("tags roundtrip: got %d, want %d", len(parsed.Tags), len(original.Tags))
	}
	if len(parsed.Related) != len(original.Related) {
		t.Errorf("related roundtrip: got %d, want %d", len(parsed.Related), len(original.Related))
	}
	if parsed.ConsolidatedAt == nil || !parsed.ConsolidatedAt.Equal(*original.ConsolidatedAt) {
		t.Errorf("consolidated_at roundtrip: got %v, want %v", parsed.ConsolidatedAt, original.ConsolidatedAt)
	}
	if !strings.Contains(parsedBody, "# Testing") {
		t.Errorf("body roundtrip: got %q", parsedBody)
	}
}

func TestStripFrontMatter(t *testing.T) {
	input := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
---

# Title

Body text.
`

	body := StripFrontMatter(input)
	if strings.Contains(body, "---") {
		t.Errorf("body should not contain front matter delimiters, got: %q", body)
	}
	if !strings.Contains(body, "# Title") {
		t.Errorf("body should contain the heading, got: %q", body)
	}
}

func TestStripFrontMatterNoFrontMatter(t *testing.T) {
	input := "# Title\n\nBody text.\n"

	body := StripFrontMatter(input)
	if body != input {
		t.Errorf("body should be unchanged, got: %q", body)
	}
}

func TestAggregateNoteMetadata(t *testing.T) {
	now := time.Now().UTC()
	notes := []model.Note{
		{
			Strength:    0.50,
			Salience:    0.60,
			DecayRate:   0.015,
			RecallCount: 3,
			Tags:        []string{"golang", "testing"},
		},
		{
			Strength:       0.72,
			Salience:       0.55,
			DecayRate:      0.010,
			RecallCount:    2,
			Tags:           []string{"testing", "ci"},
			ConsolidatedAt: &now,
		},
	}

	meta := AggregateNoteMetadata(notes)

	if meta.Strength != 0.72 {
		t.Errorf("strength: got %f, want 0.72 (max)", meta.Strength)
	}
	if meta.Salience != 0.60 {
		t.Errorf("salience: got %f, want 0.60 (max)", meta.Salience)
	}
	if meta.DecayRate != 0.010 {
		t.Errorf("decay_rate: got %f, want 0.010 (min)", meta.DecayRate)
	}
	if meta.RecallCount != 5 {
		t.Errorf("recall_count: got %d, want 5 (sum)", meta.RecallCount)
	}
	if meta.Source != "consolidation" {
		t.Errorf("source: got %q, want consolidation", meta.Source)
	}
	// Tags should be unique and sorted
	if len(meta.Tags) != 3 {
		t.Fatalf("tags: got %d, want 3 unique", len(meta.Tags))
	}
}

func TestAggregateNoteMetadataEmpty(t *testing.T) {
	meta := AggregateNoteMetadata(nil)

	if meta.DecayRate != 0.015 {
		t.Errorf("decay_rate: got %f, want 0.015 (default)", meta.DecayRate)
	}
	if meta.Source != "consolidation" {
		t.Errorf("source: got %q", meta.Source)
	}
}

func TestWriteWithMetadata(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	meta := model.KBMetadata{
		Strength:    0.72,
		Salience:    0.65,
		DecayRate:   0.015,
		RecallCount: 5,
		Source:      "consolidation",
	}
	body := "# Test\n\nContent.\n"

	if err := k.WriteWithMetadata("test.md", body, meta); err != nil {
		t.Fatalf("WriteWithMetadata: %v", err)
	}

	// Read back raw content
	raw, err := k.Read("test.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.HasPrefix(raw, "---\n") {
		t.Error("should start with front matter")
	}
	if !strings.Contains(raw, "strength: 0.72") {
		t.Error("should contain strength in front matter")
	}
	if !strings.Contains(raw, "# Test") {
		t.Error("should contain body")
	}

	// ReadBody should strip front matter
	bodyOnly, err := k.ReadBody("test.md")
	if err != nil {
		t.Fatalf("ReadBody: %v", err)
	}
	if strings.Contains(bodyOnly, "strength:") {
		t.Error("ReadBody should not contain front matter fields")
	}
	if !strings.Contains(bodyOnly, "# Test") {
		t.Error("ReadBody should contain body heading")
	}

	// ReadMetadata should parse it
	readMeta, err := k.ReadMetadata("test.md")
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if readMeta.Strength != 0.72 {
		t.Errorf("ReadMetadata strength: got %f, want 0.72", readMeta.Strength)
	}
}

func TestExtractTitleWithFrontMatter(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	content := `---
strength: 0.5
salience: 0.5
decay_rate: 0.01
recall_count: 0
---

# My Title

Content here.
`
	if err := k.Write("doc.md", content); err != nil {
		t.Fatalf("Write: %v", err)
	}

	title, err := k.ExtractTitle("doc.md")
	if err != nil {
		t.Fatalf("ExtractTitle: %v", err)
	}
	if title != "My Title" {
		t.Errorf("got title %q, want 'My Title'", title)
	}
}

func TestWriteWithMetadataNested(t *testing.T) {
	dir := t.TempDir()
	k := New(dir)

	meta := model.KBMetadata{
		Strength:  0.5,
		Salience:  0.5,
		DecayRate: 0.01,
	}

	if err := k.WriteWithMetadata(filepath.Join("sub", "doc.md"), "# Doc\n", meta); err != nil {
		t.Fatalf("WriteWithMetadata nested: %v", err)
	}

	if !k.Exists(filepath.Join("sub", "doc.md")) {
		t.Fatal("nested file should exist")
	}
}
