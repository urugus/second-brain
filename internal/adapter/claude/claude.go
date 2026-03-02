package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urugus/second-brain/internal/adapter"
)

// Agent implements adapter.Agent using the Claude Code CLI.
type Agent struct {
	executor adapter.CommandExecutor
	model    string
}

type Option func(*Agent)

func New(opts ...Option) *Agent {
	a := &Agent{
		executor: &adapter.DefaultExecutor{},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithExecutor(e adapter.CommandExecutor) Option {
	return func(a *Agent) { a.executor = e }
}

func WithModel(model string) Option {
	return func(a *Agent) { a.model = model }
}

func (a *Agent) Name() string { return "claude-code" }

// claudeJSONResponse is the envelope returned by `claude -p --output-format json`.
type claudeJSONResponse struct {
	Type             string              `json:"type"`
	Subtype          string              `json:"subtype"`
	IsError          bool                `json:"is_error"`
	Duration         int                 `json:"duration_ms"`
	NumTurns         int                 `json:"num_turns"`
	Result           string              `json:"result"`
	StructuredOutput *consolidationOutput `json:"structured_output"`
	SessionID        string              `json:"session_id"`
}

// consolidationOutput matches the JSON schema we send to Claude.
type consolidationOutput struct {
	Summary        string     `json:"summary"`
	KBUpdates      []kbUpdate `json:"kb_updates"`
	SuggestedTasks []string   `json:"suggested_tasks"`
}

type kbUpdate struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
}

func (a *Agent) Consolidate(ctx context.Context, req adapter.ConsolidationRequest) (*adapter.ConsolidationResult, error) {
	prompt := buildConsolidationPrompt(req.Session, req.Events, req.Notes, req.Tasks, req.ExistingKB)


	args := a.buildArgs(prompt)

	output, err := a.executor.Execute(ctx, "claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude consolidation failed: %w\noutput: %s", err, string(output))
	}

	return a.parseConsolidationResponse(output)
}

func (a *Agent) Summarize(ctx context.Context, text string) (string, error) {
	prompt := buildSummarizePrompt(text)

	args := []string{"-p", "--no-session-persistence"}
	if a.model != "" {
		args = append(args, "--model", a.model)
	}
	args = append(args, prompt)

	output, err := a.executor.Execute(ctx, "claude", args...)
	if err != nil {
		return "", fmt.Errorf("claude summarize failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (a *Agent) buildArgs(prompt string) []string {
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", jsonSchema,
		"--no-session-persistence",
	}
	if a.model != "" {
		args = append(args, "--model", a.model)
	}
	args = append(args, prompt)
	return args
}

func (a *Agent) parseConsolidationResponse(output []byte) (*adapter.ConsolidationResult, error) {
	// Try parsing as claude JSON envelope first
	var envelope claudeJSONResponse
	if err := json.Unmarshal(output, &envelope); err == nil {
		if envelope.IsError {
			return nil, fmt.Errorf("claude returned error: %s", envelope.Result)
		}

		// Prefer structured_output (used with --json-schema)
		if envelope.StructuredOutput != nil {
			return convertOutput(envelope.StructuredOutput), nil
		}

		// Fall back to result field
		if envelope.Result != "" {
			return a.parseResultJSON(envelope.Result)
		}
	}

	// Fallback: try parsing the output directly as our schema
	return a.parseResultJSON(string(output))
}

func convertOutput(co *consolidationOutput) *adapter.ConsolidationResult {
	result := &adapter.ConsolidationResult{
		Summary:        co.Summary,
		SuggestedTasks: co.SuggestedTasks,
	}
	for _, u := range co.KBUpdates {
		result.KBUpdates = append(result.KBUpdates, adapter.KBUpdate{
			Path:    u.Path,
			Content: u.Content,
			Reason:  u.Reason,
		})
	}
	return result
}

func (a *Agent) parseResultJSON(resultStr string) (*adapter.ConsolidationResult, error) {
	var co consolidationOutput
	if err := json.Unmarshal([]byte(resultStr), &co); err != nil {
		// Try extracting JSON block from markdown
		if extracted := extractJSONBlock(resultStr); extracted != "" {
			if err2 := json.Unmarshal([]byte(extracted), &co); err2 != nil {
				return nil, fmt.Errorf("parse consolidation result: %w (raw: %s)", err, truncate(resultStr, 200))
			}
		} else {
			return nil, fmt.Errorf("parse consolidation result: %w (raw: %s)", err, truncate(resultStr, 200))
		}
	}

	result := &adapter.ConsolidationResult{
		Summary:        co.Summary,
		SuggestedTasks: co.SuggestedTasks,
	}
	for _, u := range co.KBUpdates {
		result.KBUpdates = append(result.KBUpdates, adapter.KBUpdate{
			Path:    u.Path,
			Content: u.Content,
			Reason:  u.Reason,
		})
	}
	return result, nil
}

func extractJSONBlock(s string) string {
	start := strings.Index(s, "```json")
	if start == -1 {
		start = strings.Index(s, "```")
		if start == -1 {
			return ""
		}
	}
	// Skip past the opening fence line
	nl := strings.Index(s[start:], "\n")
	if nl == -1 {
		return ""
	}
	body := s[start+nl+1:]
	end := strings.Index(body, "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(body[:end])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
