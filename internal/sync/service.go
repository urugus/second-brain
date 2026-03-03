package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

// SyncResult is the structured output returned by Claude.
type SyncResult struct {
	Summary        string   `json:"summary"`
	NotesAdded     int      `json:"notes_added"`
	TasksAdded     int      `json:"tasks_added"`
	KBFilesUpdated []string `json:"kb_files_updated"`

	DecayedNotes   int     `json:"-"`
	PredictedNotes float64 `json:"-"`
	PredictedTasks float64 `json:"-"`
	NotesError     float64 `json:"-"`
	TasksError     float64 `json:"-"`
	PriorityDelta  int     `json:"-"`
	AdjustedTasks  int     `json:"-"`
}

// claudeJSONResponse is the envelope returned by `claude -p --output-format json`.
type claudeJSONResponse struct {
	Type             string      `json:"type"`
	Subtype          string      `json:"subtype"`
	IsError          bool        `json:"is_error"`
	Duration         int         `json:"duration_ms"`
	NumTurns         int         `json:"num_turns"`
	Result           string      `json:"result"`
	StructuredOutput *SyncResult `json:"structured_output"`
	SessionID        string      `json:"session_id"`
}

// Service orchestrates the sync process.
type Service struct {
	store    *store.Store
	executor adapter.CommandExecutor
	model    string
}

func NewService(s *store.Store, executor adapter.CommandExecutor, modelName string) *Service {
	return &Service{store: s, executor: executor, model: modelName}
}

// Run executes a single sync: call claude -p with MCP tools, parse result, log.
func (s *Service) Run(ctx context.Context) (*SyncResult, error) {
	runtimeCfg := config.LoadRuntime()
	decayedNotes, err := s.store.DecayMemories(time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("decay memories: %w", err)
	}
	profile := s.buildSyncFocusProfile(runtimeCfg)
	prompt := buildSyncPrompt(profile)
	predictedNotes, predictedTasks := s.estimateExpectedSyncOutcome(runtimeCfg.SyncPredictionWindow)

	// Create log entry
	sl, err := s.store.CreateSyncLog("claude-code", prompt)
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	// Update to running
	s.store.UpdateSyncLog(sl.ID, model.SyncRunning, "", 0, 0, "", 0, "")

	start := time.Now()

	// Build args
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", syncJSONSchema,
		"--no-session-persistence",
	}
	if s.model != "" {
		args = append(args, "--model", s.model)
	}
	args = append(args, prompt)

	// Execute claude
	output, err := s.executor.Execute(ctx, "claude", args...)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		errMsg := fmt.Sprintf("claude sync failed: %v", err)
		s.store.UpdateSyncLog(sl.ID, model.SyncFailed, "", 0, 0, "", durationMs, errMsg)
		return nil, fmt.Errorf("%s\noutput: %s", errMsg, string(output))
	}

	// Parse response
	result, err := parseSyncResponse(output)
	if err != nil {
		s.store.UpdateSyncLog(sl.ID, model.SyncFailed, "", 0, 0, "", durationMs, err.Error())
		return nil, err
	}
	result.DecayedNotes = decayedNotes
	result.PredictedNotes = predictedNotes
	result.PredictedTasks = predictedTasks
	result.NotesError = float64(result.NotesAdded) - predictedNotes
	result.TasksError = float64(result.TasksAdded) - predictedTasks
	if runtimeCfg.PredictionLearningEnabled {
		result.PriorityDelta = priorityDeltaFromError(result.TasksError)
		result.AdjustedTasks = s.applyPriorityLearning(result.PriorityDelta, runtimeCfg.PriorityAdjustLimit, profile)
		s.recordPredictionErrors(result)
	}

	// Log success
	kbFiles := strings.Join(result.KBFilesUpdated, ",")
	s.store.UpdateSyncLog(sl.ID, model.SyncCompleted, result.Summary, result.NotesAdded, result.TasksAdded, kbFiles, durationMs, "")

	return result, nil
}

func parseSyncResponse(output []byte) (*SyncResult, error) {
	// Try parsing as Claude JSON envelope
	var envelope claudeJSONResponse
	if err := json.Unmarshal(output, &envelope); err == nil {
		if envelope.IsError {
			return nil, fmt.Errorf("claude returned error: %s", envelope.Result)
		}
		if envelope.StructuredOutput != nil {
			return envelope.StructuredOutput, nil
		}
		if envelope.Result != "" {
			var result SyncResult
			if err := json.Unmarshal([]byte(envelope.Result), &result); err == nil {
				return &result, nil
			}
		}
	}

	// Fallback: try parsing directly
	var result SyncResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse sync response: %w (raw: %s)", err, truncate(string(output), 200))
	}
	return &result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (s *Service) estimateExpectedSyncOutcome(window int) (float64, float64) {
	predictedNotes, predictedTasks, err := s.store.EstimateSyncPrediction(window)
	if err != nil {
		return 0, 0
	}
	return predictedNotes, predictedTasks
}

func (s *Service) applyPriorityLearning(priorityDelta int, limit int, profile *focusProfile) int {
	adjusted, err := s.store.AdjustTodoTaskPriorities(
		priorityDelta,
		limit,
		priorityLearningContextTerms(profile),
	)
	if err != nil {
		return 0
	}
	return adjusted
}

func priorityLearningContextTerms(profile *focusProfile) []string {
	if profile == nil {
		return nil
	}
	terms := make([]string, 0, len(profile.Terms)+len(profile.Tags))
	terms = append(terms, profile.Terms...)
	terms = append(terms, profile.Tags...)
	return terms
}

func (s *Service) recordPredictionErrors(result *SyncResult) {
	_ = s.store.RecordPredictionError(
		model.PredictionSourceSync,
		"notes_added",
		result.PredictedNotes,
		float64(result.NotesAdded),
		0,
	)
	_ = s.store.RecordPredictionError(
		model.PredictionSourceSync,
		"tasks_added",
		result.PredictedTasks,
		float64(result.TasksAdded),
		result.PriorityDelta,
	)
}

func priorityDeltaFromError(tasksError float64) int {
	switch {
	case tasksError >= 3:
		return 2
	case tasksError >= 1:
		return 1
	case tasksError <= -3:
		return -2
	case tasksError <= -1:
		return -1
	default:
		return 0
	}
}
