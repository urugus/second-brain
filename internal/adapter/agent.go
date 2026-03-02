package adapter

import (
	"context"

	"github.com/urugus/second-brain/internal/model"
)

// ConsolidationRequest contains the data an agent needs to perform consolidation.
type ConsolidationRequest struct {
	Session    *model.Session
	Notes      []model.Note
	Tasks      []model.Task
	Events     []model.Event
	ExistingKB []string // paths to relevant existing KB files
}

// KBUpdate represents a single knowledge base file update.
type KBUpdate struct {
	Path    string
	Content string
	Reason  string
}

// ConsolidationResult contains what the agent produced.
type ConsolidationResult struct {
	Summary        string
	KBUpdates      []KBUpdate
	SuggestedTasks []string
}

// Agent is the interface for LLM-based consolidation agents.
// An agent takes working memory data and produces long-term memory updates.
type Agent interface {
	Name() string
	Consolidate(ctx context.Context, req ConsolidationRequest) (*ConsolidationResult, error)
	Summarize(ctx context.Context, text string) (string, error)
}
