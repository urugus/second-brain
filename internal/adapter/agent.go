package adapter

import "context"

// ConsolidationRequest contains the data an agent needs to perform consolidation.
type ConsolidationRequest struct {
	SessionID  int64
	Notes      []string
	Tasks      []string
	Events     []string // JSON-encoded event payloads in chronological order
	ExistingKB []string // paths to relevant existing KB files
}

// ConsolidationResult contains what the agent produced.
type ConsolidationResult struct {
	Summary        string
	KBUpdates      map[string]string // filepath -> new markdown content
	SuggestedTasks []string
}

// Agent is the interface for LLM-based consolidation agents.
// An agent takes working memory data and produces long-term memory updates.
type Agent interface {
	Name() string
	Consolidate(ctx context.Context, req ConsolidationRequest) (*ConsolidationResult, error)
	Summarize(ctx context.Context, text string) (string, error)
}
