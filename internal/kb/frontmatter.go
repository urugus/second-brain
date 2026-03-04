package kb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/model"
)

const frontMatterDelimiter = "---"

// ParseFrontMatter splits a markdown document into its YAML front matter
// metadata and body content. If no front matter is present, returns a zero
// KBMetadata and the original content.
func ParseFrontMatter(content string) (model.KBMetadata, string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != frontMatterDelimiter {
		return model.KBMetadata{}, content
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontMatterDelimiter {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return model.KBMetadata{}, content
	}

	yamlLines := lines[1:endIdx]
	meta := parseYAMLFields(yamlLines)

	// Body starts after the closing delimiter; skip one leading blank line.
	bodyStart := endIdx + 1
	if bodyStart < len(lines) && strings.TrimSpace(lines[bodyStart]) == "" {
		bodyStart++
	}
	body := strings.Join(lines[bodyStart:], "\n")
	return meta, body
}

// MarshalFrontMatter produces a complete markdown document with YAML front
// matter followed by the body content.
func MarshalFrontMatter(meta model.KBMetadata, body string) string {
	var b strings.Builder
	b.WriteString(frontMatterDelimiter)
	b.WriteByte('\n')

	writeFloat := func(key string, v float64) {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		b.WriteByte('\n')
	}

	writeFloat("strength", meta.Strength)
	writeFloat("salience", meta.Salience)
	writeFloat("decay_rate", meta.DecayRate)

	b.WriteString("recall_count: ")
	b.WriteString(strconv.Itoa(meta.RecallCount))
	b.WriteByte('\n')

	if meta.Source != "" {
		b.WriteString("source: ")
		b.WriteString(meta.Source)
		b.WriteByte('\n')
	}

	if len(meta.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range meta.Tags {
			b.WriteString("  - ")
			b.WriteString(tag)
			b.WriteByte('\n')
		}
	}

	if meta.ConsolidatedAt != nil {
		b.WriteString("consolidated_at: ")
		b.WriteString(meta.ConsolidatedAt.Format(time.RFC3339))
		b.WriteByte('\n')
	}

	if len(meta.Related) > 0 {
		b.WriteString("related:\n")
		for _, r := range meta.Related {
			b.WriteString("  - path: ")
			b.WriteString(r.Path)
			b.WriteByte('\n')
			b.WriteString("    weight: ")
			b.WriteString(strconv.FormatFloat(r.Weight, 'f', -1, 64))
			b.WriteByte('\n')
		}
	}

	b.WriteString(frontMatterDelimiter)
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(body)
	return b.String()
}

// StripFrontMatter removes YAML front matter from content, returning only the
// body. Useful when the caller only needs the readable markdown.
func StripFrontMatter(content string) string {
	_, body := ParseFrontMatter(content)
	return body
}

// parseYAMLFields is a minimal YAML parser sufficient for our front matter
// schema. It handles scalar fields, a list of strings (tags), and a list of
// objects (related entries).
func parseYAMLFields(lines []string) model.KBMetadata {
	var meta model.KBMetadata

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		key, val := splitKeyValue(trimmed)
		switch key {
		case "strength":
			meta.Strength = parseFloat(val)
		case "salience":
			meta.Salience = parseFloat(val)
		case "decay_rate":
			meta.DecayRate = parseFloat(val)
		case "recall_count":
			meta.RecallCount = parseInt(val)
		case "source":
			meta.Source = val
		case "consolidated_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				meta.ConsolidatedAt = &t
			}
		case "tags":
			if val == "" {
				// Inline list items follow
				i++
				for i < len(lines) {
					item := strings.TrimSpace(lines[i])
					if !strings.HasPrefix(item, "- ") {
						break
					}
					meta.Tags = append(meta.Tags, strings.TrimSpace(strings.TrimPrefix(item, "- ")))
					i++
				}
				continue
			}
			// Inline: tags: [a, b, c]
			meta.Tags = parseInlineList(val)
		case "related":
			i++
			for i < len(lines) {
				item := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(item, "- ") {
					break
				}
				entry := model.KBRelatedEntry{}
				// Parse "- path: xxx"
				pathVal := strings.TrimSpace(strings.TrimPrefix(item, "- "))
				pk, pv := splitKeyValue(pathVal)
				if pk == "path" {
					entry.Path = pv
				}
				i++
				// Parse "  weight: xxx"
				if i < len(lines) {
					wLine := strings.TrimSpace(lines[i])
					wk, wv := splitKeyValue(wLine)
					if wk == "weight" {
						entry.Weight = parseFloat(wv)
						i++
					}
				}
				meta.Related = append(meta.Related, entry)
			}
			continue
		}
		i++
	}

	return meta
}

func splitKeyValue(s string) (string, string) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func parseInlineList(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ImportMeta is a lightweight struct used during the kb import command to carry
// related entries through a two-phase import process.
type ImportMeta struct {
	Related []model.KBRelatedEntry
}

// AggregateNoteMetadata computes a KBMetadata from a set of source notes.
// It takes the maximum strength/salience, sums recall counts, and collects
// unique tags.
func AggregateNoteMetadata(notes []model.Note) model.KBMetadata {
	if len(notes) == 0 {
		return model.KBMetadata{
			DecayRate: 0.015,
			Source:    "consolidation",
		}
	}

	var meta model.KBMetadata
	meta.Source = "consolidation"
	meta.DecayRate = notes[0].DecayRate

	tagSet := make(map[string]struct{})
	var latestConsolidated *time.Time

	for _, n := range notes {
		if n.Strength > meta.Strength {
			meta.Strength = n.Strength
		}
		if n.Salience > meta.Salience {
			meta.Salience = n.Salience
		}
		meta.RecallCount += n.RecallCount
		if n.DecayRate < meta.DecayRate {
			meta.DecayRate = n.DecayRate
		}
		for _, tag := range n.Tags {
			tagSet[tag] = struct{}{}
		}
		if n.ConsolidatedAt != nil {
			if latestConsolidated == nil || n.ConsolidatedAt.After(*latestConsolidated) {
				latestConsolidated = n.ConsolidatedAt
			}
		}
	}

	for tag := range tagSet {
		if tag != "" {
			meta.Tags = append(meta.Tags, tag)
		}
	}
	// Sort tags for deterministic output
	sortStrings(meta.Tags)

	now := time.Now().UTC()
	if latestConsolidated != nil {
		meta.ConsolidatedAt = latestConsolidated
	} else {
		meta.ConsolidatedAt = &now
	}

	return meta
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// BuildMetadataForKBWrite creates a complete KBMetadata including related
// entries from the store's RelatedKBFiles query.
func BuildMetadataForKBWrite(notes []model.Note, relatedFiles []model.RelatedKBFile) model.KBMetadata {
	meta := AggregateNoteMetadata(notes)
	for _, r := range relatedFiles {
		meta.Related = append(meta.Related, model.KBRelatedEntry{
			Path:   r.Path,
			Weight: roundFloat(r.Weight, 4),
		})
	}
	return meta
}

func roundFloat(v float64, decimals int) float64 {
	shift := 1.0
	for i := 0; i < decimals; i++ {
		shift *= 10
	}
	return float64(int(v*shift+0.5)) / shift
}

// FormatMetadataInfo returns a human-readable summary of KB metadata.
func FormatMetadataInfo(meta model.KBMetadata) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  strength=%.2f  salience=%.2f  decay_rate=%.3f  recall_count=%d",
		meta.Strength, meta.Salience, meta.DecayRate, meta.RecallCount))
	if meta.Source != "" {
		b.WriteString(fmt.Sprintf("  source=%s", meta.Source))
	}
	if len(meta.Tags) > 0 {
		b.WriteString(fmt.Sprintf("  tags=[%s]", strings.Join(meta.Tags, ",")))
	}
	if len(meta.Related) > 0 {
		b.WriteString(fmt.Sprintf("  related=%d files", len(meta.Related)))
	}
	return b.String()
}
