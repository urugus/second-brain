package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/consolidation"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

// AgentFactory creates an adapter.Agent, optionally configured with a model.
type AgentFactory func(model string) adapter.Agent

type consolidateInput struct {
	SessionID *int64 `json:"session_id,omitempty" jsonschema:"Session ID to consolidate (default: latest unconsolidated completed/abandoned session)"`
	Mode      string `json:"mode,omitempty" jsonschema:"Mode: propose (dry run, default) or apply (propose and auto-apply all changes)"`
	Model     string `json:"model,omitempty" jsonschema:"Claude model to use for consolidation"`
}

func registerConsolidationTools(server *gomcp.Server, s *store.Store, k *kb.KB, factory AgentFactory) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "consolidate",
		Description: "Consolidate a completed session's knowledge into the knowledge base. Use mode 'propose' (default) to preview changes, or 'apply' to propose and auto-apply all changes.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input consolidateInput) (*gomcp.CallToolResult, any, error) {
		mode := input.Mode
		if mode == "" {
			mode = "propose"
		}
		if mode != "propose" && mode != "apply" {
			return errResult(fmt.Sprintf("invalid mode %q: must be 'propose' or 'apply'", mode)), nil, nil
		}

		// Determine session ID
		sessionID := int64(0)
		if input.SessionID != nil {
			sessionID = *input.SessionID
		}
		if sessionID == 0 {
			sess, err := s.LatestUnconsolidatedSession()
			if err != nil {
				return errResult("failed to find session: " + err.Error()), nil, nil
			}
			if sess == nil {
				return errResult("no unconsolidated sessions found"), nil, nil
			}
			sessionID = sess.ID
		}

		// Create agent and service
		agent := factory(input.Model)
		svc := consolidation.NewService(s, k, agent)

		// Propose
		proposed, err := svc.Propose(ctx, sessionID)
		if err != nil {
			return errResult("consolidation propose failed: " + err.Error()), nil, nil
		}

		if mode == "apply" {
			// Auto-approve all changes
			kbIndices := make([]int, len(proposed.KBUpdates))
			for i := range kbIndices {
				kbIndices[i] = i
			}
			taskIndices := make([]int, len(proposed.SuggestedTasks))
			for i := range taskIndices {
				taskIndices[i] = i
			}

			if err := svc.Apply(ctx, proposed, kbIndices, taskIndices); err != nil {
				return errResult("consolidation apply failed: " + err.Error()), nil, nil
			}

			r, err := jsonResult(map[string]any{
				"status":         "applied",
				"session_id":     sessionID,
				"summary":        proposed.Summary,
				"kb_files":       len(proposed.KBUpdates),
				"tasks_created":  len(proposed.SuggestedTasks),
				"kb_updates":     proposed.KBUpdates,
				"suggested_tasks": proposed.SuggestedTasks,
			})
			return r, nil, err
		}

		// Propose-only mode
		r, err := jsonResult(map[string]any{
			"status":         "proposed",
			"session_id":     sessionID,
			"summary":        proposed.Summary,
			"kb_updates":     proposed.KBUpdates,
			"suggested_tasks": proposed.SuggestedTasks,
		})
		return r, nil, err
	})
}
