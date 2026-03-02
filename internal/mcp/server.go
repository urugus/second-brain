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

// serverInstructions describes the server and its capabilities for MCP clients.
const serverInstructions = `second-brain is a personal knowledge management system that captures work sessions, tasks, notes, and long-term knowledge.

Available tool categories:
- Session management: start_session, end_session, get_active_session, list_sessions
- Task tracking: create_task, list_tasks, update_task_status
- Note taking: create_note, list_notes
- Knowledge base: kb_list, kb_read, kb_search, kb_write
- Event tracking: list_events
- Consolidation: consolidate

Typical workflow:
1. Start a session with start_session (title and optional goal).
2. During work, create tasks and notes. They auto-attach to the active session.
3. End the session with end_session (provide a summary).
4. Run consolidate to extract knowledge into the KB from the completed session.

Important conventions:
- Only one session can be active at a time. End or abandon the current session before starting a new one.
- Tasks and notes auto-attach to the active session when no session_id is provided.
- The knowledge base stores persistent markdown files organized by topic (e.g. "go/testing.md").
- Use consolidate in "propose" mode first to preview changes, then "apply" to commit them.`

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
	}, &gomcp.ServerOptions{
		Instructions: serverInstructions,
	})

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
