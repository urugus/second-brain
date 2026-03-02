package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

type getActiveSessionInput struct{}

type listSessionsInput struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by status (active, completed, abandoned)"`
}

func registerSessionTools(server *gomcp.Server, s *store.Store) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_active_session",
		Description: "Get the currently active work session, if any",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input getActiveSessionInput) (*gomcp.CallToolResult, any, error) {
		sess, err := s.ActiveSession()
		if err != nil {
			return errResult("failed to get active session: " + err.Error()), nil, nil
		}
		if sess == nil {
			return textResult("No active session"), nil, nil
		}
		r, err := jsonResult(sess)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_sessions",
		Description: "List work sessions, optionally filtered by status (active, completed, abandoned)",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input listSessionsInput) (*gomcp.CallToolResult, any, error) {
		var statusFilter *model.SessionStatus
		if input.Status != "" {
			st := model.SessionStatus(input.Status)
			statusFilter = &st
		}
		sessions, err := s.ListSessions(statusFilter)
		if err != nil {
			return errResult("failed to list sessions: " + err.Error()), nil, nil
		}
		if len(sessions) == 0 {
			return textResult("No sessions found"), nil, nil
		}
		r, err := jsonResult(sessions)
		return r, nil, err
	})
}
