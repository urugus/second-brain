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

type startSessionInput struct {
	Title string `json:"title" jsonschema:"Session title"`
	Goal  string `json:"goal,omitempty" jsonschema:"Session goal"`
}

type endSessionInput struct {
	Summary string `json:"summary,omitempty" jsonschema:"Session summary"`
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

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "start_session",
		Description: "Start a new work session",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input startSessionInput) (*gomcp.CallToolResult, any, error) {
		sess, err := s.CreateSession(input.Title, input.Goal)
		if err != nil {
			return errResult("failed to start session: " + err.Error()), nil, nil
		}
		r, err := jsonResult(sess)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "end_session",
		Description: "End the current active session",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input endSessionInput) (*gomcp.CallToolResult, any, error) {
		sess, err := s.ActiveSession()
		if err != nil {
			return errResult("failed to get active session: " + err.Error()), nil, nil
		}
		if sess == nil {
			return errResult("no active session"), nil, nil
		}
		if err := s.EndSession(sess.ID, input.Summary); err != nil {
			return errResult("failed to end session: " + err.Error()), nil, nil
		}
		// Re-fetch the session to get the updated state
		ended, err := s.GetSession(sess.ID)
		if err != nil {
			return errResult("failed to get ended session: " + err.Error()), nil, nil
		}
		r, err := jsonResult(ended)
		return r, nil, err
	})
}
