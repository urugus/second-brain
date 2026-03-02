package consolidation

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

const relatedSectionHeader = "## Related"
const maxRelatedLinks = 5

func stripRelatedSection(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inRelated := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == relatedSectionHeader {
			inRelated = true
			// Strip preceding "---" separator and blank lines
			for len(result) > 0 {
				last := strings.TrimSpace(result[len(result)-1])
				if last == "" || last == "---" {
					result = result[:len(result)-1]
				} else {
					break
				}
			}
			continue
		}

		if inRelated {
			if strings.HasPrefix(trimmed, "## ") {
				inRelated = false
				// Restore blank line before next heading
				result = append(result, "")
				result = append(result, line)
			}
			continue
		}

		result = append(result, line)
	}

	text := strings.Join(result, "\n")
	return strings.TrimRight(text, "\n") + "\n"
}

func buildRelatedSection(kbPath string, s *store.Store, k *kb.KB) string {
	related, err := s.RelatedKBFiles(kbPath, maxRelatedLinks)
	if err != nil || len(related) == 0 {
		return ""
	}

	fromDir := filepath.Dir(kbPath)

	var links []string
	for _, r := range related {
		if !k.Exists(r.Path) {
			continue
		}
		title, err := k.ExtractTitle(r.Path)
		if err != nil {
			continue
		}
		relPath, err := filepath.Rel(fromDir, r.Path)
		if err != nil {
			relPath = r.Path
		}
		links = append(links, fmt.Sprintf("- [%s](%s)", title, relPath))
	}

	if len(links) == 0 {
		return ""
	}

	return "\n---\n\n" + relatedSectionHeader + "\n" + strings.Join(links, "\n") + "\n"
}
