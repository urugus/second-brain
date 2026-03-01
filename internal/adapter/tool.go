package adapter

import "context"

// Message represents a single message captured from an external tool.
type Message struct {
	Source   string
	Content  string
	Metadata map[string]string
}

// Tool is the interface for external input sources.
// Each tool adapter captures information from an external system
// and converts it into Messages that can be stored as notes.
type Tool interface {
	Name() string
	Capture(ctx context.Context) ([]Message, error)
	Health(ctx context.Context) error
}
