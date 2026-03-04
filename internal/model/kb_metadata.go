package model

import "time"

// KBMetadata holds portable weighting metadata embedded as YAML front matter
// in knowledge base markdown files. This enables knowledge portability across
// environments without relying on the SQLite database for weight information.
type KBMetadata struct {
	Strength       float64          `yaml:"strength"`
	Salience       float64          `yaml:"salience"`
	DecayRate      float64          `yaml:"decay_rate"`
	RecallCount    int              `yaml:"recall_count"`
	Source         string           `yaml:"source"`
	Tags           []string         `yaml:"tags,omitempty"`
	ConsolidatedAt *time.Time       `yaml:"consolidated_at,omitempty"`
	Related        []KBRelatedEntry `yaml:"related,omitempty"`
}

// KBRelatedEntry represents a weighted relationship to another KB file.
type KBRelatedEntry struct {
	Path   string  `yaml:"path"`
	Weight float64 `yaml:"weight"`
}
