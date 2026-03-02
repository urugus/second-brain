package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/store"
)

type createNoteInput struct {
	Content   string   `json:"content" jsonschema:"Note content"`
	Tags      []string `json:"tags,omitempty" jsonschema:"Tags for categorization"`
	Source    string   `json:"source,omitempty" jsonschema:"Source of the note (e.g. claude-code)"`
	SessionID *int64   `json:"session_id,omitempty" jsonschema:"Session to attach to (defaults to active session)"`
}

type listNotesInput struct {
	SessionID *int64 `json:"session_id,omitempty" jsonschema:"Filter by session ID"`
	Tag       string `json:"tag,omitempty" jsonschema:"Filter by tag"`
}

func registerNoteTools(server *gomcp.Server, s *store.Store) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_note",
		Description: "Create a new note. Auto-attaches to the active session if no session_id is provided.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input createNoteInput) (*gomcp.CallToolResult, any, error) {
		sessionID := input.SessionID
		if sessionID == nil {
			if sess, err := s.ActiveSession(); err == nil && sess != nil {
				sessionID = &sess.ID
			}
		}
		note, err := s.CreateNote(input.Content, sessionID, input.Tags, input.Source)
		if err != nil {
			return errResult("failed to create note: " + err.Error()), nil, nil
		}
		r, err := jsonResult(note)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_notes",
		Description: "List notes, optionally filtered by session ID and/or tag",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input listNotesInput) (*gomcp.CallToolResult, any, error) {
		filter := store.NoteFilter{SessionID: input.SessionID}
		if input.Tag != "" {
			filter.Tag = &input.Tag
		}
		notes, err := s.ListNotes(filter)
		if err != nil {
			return errResult("failed to list notes: " + err.Error()), nil, nil
		}
		if len(notes) == 0 {
			return textResult("No notes found"), nil, nil
		}
		r, err := jsonResult(notes)
		return r, nil, err
	})
}
