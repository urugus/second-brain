package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

type createTaskInput struct {
	Title       string `json:"title" jsonschema:"Task title"`
	Description string `json:"description,omitempty" jsonschema:"Task description"`
	Priority    int    `json:"priority,omitempty" jsonschema:"Priority (0=none 1=low 2=medium 3=high)"`
	SessionID   *int64 `json:"session_id,omitempty" jsonschema:"Session to attach to (defaults to active session)"`
}

type listTasksInput struct {
	Status    string `json:"status,omitempty" jsonschema:"Filter by status (todo, in_progress, done, cancelled)"`
	SessionID *int64 `json:"session_id,omitempty" jsonschema:"Filter by session ID"`
}

type updateTaskStatusInput struct {
	ID     int64  `json:"id" jsonschema:"Task ID"`
	Status string `json:"status" jsonschema:"New status (todo, in_progress, done, cancelled)"`
}

func registerTaskTools(server *gomcp.Server, s *store.Store) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_task",
		Description: "Create a new task. Auto-attaches to the active session if no session_id is provided.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input createTaskInput) (*gomcp.CallToolResult, any, error) {
		sessionID := input.SessionID
		if sessionID == nil {
			if sess, err := s.ActiveSession(); err == nil && sess != nil {
				sessionID = &sess.ID
			}
		}
		task, err := s.CreateTask(input.Title, input.Description, sessionID, input.Priority)
		if err != nil {
			return errResult("failed to create task: " + err.Error()), nil, nil
		}
		r, err := jsonResult(task)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks, optionally filtered by status (todo, in_progress, done, cancelled) and/or session ID",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input listTasksInput) (*gomcp.CallToolResult, any, error) {
		filter := store.TaskFilter{SessionID: input.SessionID}
		if input.Status != "" {
			st := model.TaskStatus(input.Status)
			filter.Status = &st
		}
		tasks, err := s.ListTasks(filter)
		if err != nil {
			return errResult("failed to list tasks: " + err.Error()), nil, nil
		}
		if len(tasks) == 0 {
			return textResult("No tasks found"), nil, nil
		}
		r, err := jsonResult(tasks)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_task_status",
		Description: "Update a task's status (todo, in_progress, done, cancelled)",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input updateTaskStatusInput) (*gomcp.CallToolResult, any, error) {
		err := s.UpdateTaskStatus(input.ID, model.TaskStatus(input.Status))
		if err != nil {
			return errResult("failed to update task status: " + err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("Task %d status updated to %s", input.ID, input.Status)), nil, nil
	})
}
