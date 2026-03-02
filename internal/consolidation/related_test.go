package consolidation

import (
	"testing"
)

func TestStripRelatedSection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no related section",
			input: "# Title\n\nSome content.\n",
			want:  "# Title\n\nSome content.\n",
		},
		{
			name:  "related section at end",
			input: "# Title\n\nContent.\n\n---\n\n## Related\n- [Foo](foo.md)\n- [Bar](bar.md)\n",
			want:  "# Title\n\nContent.\n",
		},
		{
			name:  "related section with trailing newlines",
			input: "# Title\n\nContent.\n\n\n---\n\n## Related\n- [Foo](foo.md)\n",
			want:  "# Title\n\nContent.\n",
		},
		{
			name:  "related section followed by another heading",
			input: "# Title\n\n## Related\n- [Foo](foo.md)\n\n## Other\nMore content.\n",
			want:  "# Title\n\n## Other\nMore content.\n",
		},
		{
			name:  "content only",
			input: "Just text.\n",
			want:  "Just text.\n",
		},
		{
			name:  "related section followed by h1 heading",
			input: "# Title\n\n## Related\n- [Foo](foo.md)\n\n# Appendix\nAppendix content.\n",
			want:  "# Title\n\n# Appendix\nAppendix content.\n",
		},
		{
			name:  "related section followed by h3 heading",
			input: "# Title\n\n## Related\n- [Foo](foo.md)\n\n### Notes\nSome notes.\n",
			want:  "# Title\n\n### Notes\nSome notes.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripRelatedSection(tt.input)
			if got != tt.want {
				t.Errorf("stripRelatedSection:\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestBuildRelatedSection(t *testing.T) {
	s, k := setupTest(t)

	// Create KB files
	k.Write("topic/main.md", "# Main Topic\nContent.\n")
	k.Write("topic/related1.md", "# Related One\nContent.\n")
	k.Write("topic/related2.md", "# Related Two\nContent.\n")

	// Create notes and edges
	nMain, _ := s.CreateNote("main note", nil, nil, "")
	nRel1, _ := s.CreateNote("related note 1", nil, nil, "")
	nRel2, _ := s.CreateNote("related note 2", nil, nil, "")

	s.LinkNotes(nMain.ID, nRel1.ID, 0.9, "main->rel1")
	s.LinkNotes(nMain.ID, nRel2.ID, 0.5, "main->rel2")

	// Map notes to KB files
	s.MapKBNotes("topic/main.md", []int64{nMain.ID})
	s.MapKBNotes("topic/related1.md", []int64{nRel1.ID})
	s.MapKBNotes("topic/related2.md", []int64{nRel2.ID})

	section := buildRelatedSection("topic/main.md", s, k)

	if section == "" {
		t.Fatal("expected non-empty related section")
	}

	// Should contain links to both related files
	if !contains(section, "Related One") {
		t.Errorf("expected 'Related One' in section: %s", section)
	}
	if !contains(section, "Related Two") {
		t.Errorf("expected 'Related Two' in section: %s", section)
	}
	if !contains(section, "## Related") {
		t.Errorf("expected '## Related' header: %s", section)
	}
}

func TestBuildRelatedSectionNoRelated(t *testing.T) {
	s, k := setupTest(t)

	k.Write("isolated.md", "# Isolated\nNo friends.\n")
	n, _ := s.CreateNote("lonely", nil, nil, "")
	s.MapKBNotes("isolated.md", []int64{n.ID})

	section := buildRelatedSection("isolated.md", s, k)
	if section != "" {
		t.Fatalf("expected empty section for isolated file, got %q", section)
	}
}

func TestBuildRelatedSectionCrossDirectory(t *testing.T) {
	s, k := setupTest(t)

	// Files in different directories
	k.Write("golang/tips.md", "# Go Tips\n")
	k.Write("concepts/design.md", "# Design Patterns\n")

	nGo, _ := s.CreateNote("go stuff", nil, nil, "")
	nDesign, _ := s.CreateNote("design stuff", nil, nil, "")

	s.LinkNotes(nGo.ID, nDesign.ID, 0.7, "go->design")
	s.MapKBNotes("golang/tips.md", []int64{nGo.ID})
	s.MapKBNotes("concepts/design.md", []int64{nDesign.ID})

	section := buildRelatedSection("golang/tips.md", s, k)

	// Should contain a relative path with ../
	if !contains(section, "../concepts/design.md") {
		t.Errorf("expected relative cross-directory path: %s", section)
	}
	if !contains(section, "Design Patterns") {
		t.Errorf("expected title 'Design Patterns': %s", section)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

