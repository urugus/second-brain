package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/store"
)

type listEventsInput struct {
	SessionID int64 `json:"session_id" jsonschema:"Session ID to list events for"`
}

func registerEventTools(server *gomcp.Server, s *store.Store) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_events",
		Description: "List events for a session in chronological order",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input listEventsInput) (*gomcp.CallToolResult, any, error) {
		events, err := s.ListEventsBySession(input.SessionID)
		if err != nil {
			return errResult("failed to list events: " + err.Error()), nil, nil
		}
		if len(events) == 0 {
			return textResult("No events found for this session"), nil, nil
		}
		r, err := jsonResult(events)
		return r, nil, err
	})
}
