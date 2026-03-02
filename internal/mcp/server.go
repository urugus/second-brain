package mcp

import (
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/adapter"
	claudeAdapter "github.com/urugus/second-brain/internal/adapter/claude"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

// Option configures the MCP server.
type Option func(*serverConfig)

type serverConfig struct {
	agentFactory AgentFactory
}

// WithAgentFactory overrides the default AgentFactory used for consolidation.
// This is primarily useful for testing.
func WithAgentFactory(f AgentFactory) Option {
	return func(c *serverConfig) { c.agentFactory = f }
}

func defaultAgentFactory(model string) adapter.Agent {
	var opts []claudeAdapter.Option
	if model != "" {
		opts = append(opts, claudeAdapter.WithModel(model))
	}
	return claudeAdapter.New(opts...)
}

// New creates a configured MCP server with all second-brain tools registered.
func New(s *store.Store, k *kb.KB, opts ...Option) *gomcp.Server {
	cfg := &serverConfig{
		agentFactory: defaultAgentFactory,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "second-brain",
		Version: "0.1.0",
	}, nil)

	registerSessionTools(server, s)
	registerTaskTools(server, s)
	registerNoteTools(server, s)
	registerKBTools(server, k)
	registerEventTools(server, s)
	registerConsolidationTools(server, s, k, cfg.agentFactory)

	return server
}

// textResult creates a CallToolResult with a plain text message.
func textResult(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}
}

// jsonResult creates a CallToolResult with JSON-formatted text content.
func jsonResult(v any) (*gomcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
	}, nil
}

// errResult creates an error CallToolResult. Tool-level errors are returned
// as content with IsError set, not as Go errors (which indicate protocol failures).
func errResult(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}
}
