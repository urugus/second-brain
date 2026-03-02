package consolidation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

// Service orchestrates the consolidation process.
// It gathers session data, calls the agent, and applies approved changes.
type Service struct {
	store *store.Store
	kb    *kb.KB
	agent adapter.Agent
}

func NewService(s *store.Store, k *kb.KB, a adapter.Agent) *Service {
	return &Service{store: s, kb: k, agent: a}
}

// ProposedChanges holds what the agent wants to do, before user approval.
type ProposedChanges struct {
	LogID          int64
	SessionID      int64
	Summary        string
	KBUpdates      []KBUpdateProposal
	SuggestedTasks []string
}

// KBUpdateProposal is a proposed KB file change with context.
type KBUpdateProposal struct {
	Path       string
	Content    string
	Reason     string
	IsNew      bool
	OldContent string // non-empty only if IsNew is false
}

// Propose gathers session data, calls the agent, and returns proposed changes.
func (s *Service) Propose(ctx context.Context, sessionID int64) (*ProposedChanges, error) {
	// Validate session
	session, err := s.store.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session.Status == model.SessionActive {
		return nil, fmt.Errorf("session #%d is still active; end it first", sessionID)
	}

	// Create consolidation log
	cl, err := s.store.CreateConsolidationLog(sessionID, s.agent.Name())
	if err != nil {
		return nil, fmt.Errorf("create consolidation log: %w", err)
	}

	// Gather data
	tasks, err := s.store.ListTasks(store.TaskFilter{SessionID: &sessionID})
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("gather tasks: %v", err), "")
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	notes, err := s.store.ListNotes(store.NoteFilter{SessionID: &sessionID})
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("gather notes: %v", err), "")
		return nil, fmt.Errorf("list notes: %w", err)
	}

	events, err := s.store.ListEventsBySession(sessionID)
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("gather events: %v", err), "")
		return nil, fmt.Errorf("list events: %w", err)
	}

	kbFiles, err := s.kb.List()
	if err != nil {
		kbFiles = []string{} // non-fatal: KB might be empty
	}

	// Update log to running
	s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationRunning, "", "")

	// Call agent
	req := adapter.ConsolidationRequest{
		Mode:       "session",
		Session:    session,
		Notes:      notes,
		Tasks:      tasks,
		Events:     events,
		ExistingKB: kbFiles,
	}

	result, err := s.agent.Consolidate(ctx, req)
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("agent error: %v", err), "")
		return nil, fmt.Errorf("agent consolidation: %w", err)
	}

	// Enrich KB updates with existing content
	var proposals []KBUpdateProposal
	for _, u := range result.KBUpdates {
		proposal := KBUpdateProposal{
			Path:    u.Path,
			Content: u.Content,
			Reason:  u.Reason,
			IsNew:   !s.kb.Exists(u.Path),
		}
		if !proposal.IsNew {
			if old, err := s.kb.Read(u.Path); err == nil {
				proposal.OldContent = old
			}
		}
		proposals = append(proposals, proposal)
	}

	return &ProposedChanges{
		LogID:          cl.ID,
		SessionID:      sessionID,
		Summary:        result.Summary,
		KBUpdates:      proposals,
		SuggestedTasks: result.SuggestedTasks,
	}, nil
}

// SleepResult holds the outcome of a sleep-mode consolidation.
type SleepResult struct {
	LogID          int64
	NotesProcessed int
	Summary        string
	KBFilesUpdated []string
	TasksCreated   int
}

// SleepConsolidate runs autonomous consolidation on unconsolidated notes.
// Returns nil, nil if unconsolidated notes are below the threshold.
func (s *Service) SleepConsolidate(ctx context.Context, threshold int) (*SleepResult, error) {
	count, err := s.store.CountUnconsolidatedNotes()
	if err != nil {
		return nil, fmt.Errorf("count unconsolidated notes: %w", err)
	}
	if count < threshold {
		return nil, nil
	}

	notes, err := s.store.ListNotes(store.NoteFilter{Unconsolidated: true})
	if err != nil {
		return nil, fmt.Errorf("list unconsolidated notes: %w", err)
	}
	if len(notes) == 0 {
		return nil, nil
	}

	cl, err := s.store.CreateSleepConsolidationLog(s.agent.Name())
	if err != nil {
		return nil, fmt.Errorf("create consolidation log: %w", err)
	}

	kbFiles, err := s.kb.List()
	if err != nil {
		kbFiles = []string{}
	}

	s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationRunning, "", "")

	req := adapter.ConsolidationRequest{
		Mode:       "sleep",
		Notes:      notes,
		ExistingKB: kbFiles,
	}

	result, err := s.agent.Consolidate(ctx, req)
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("agent error: %v", err), "")
		return nil, fmt.Errorf("agent consolidation: %w", err)
	}

	var appliedFiles []string
	for _, u := range result.KBUpdates {
		if err := s.kb.Write(u.Path, u.Content); err != nil {
			continue
		}
		appliedFiles = append(appliedFiles, u.Path)
	}

	for _, taskTitle := range result.SuggestedTasks {
		s.store.CreateTask(taskTitle, "", nil, 0)
	}

	noteIDs := make([]int64, len(notes))
	for i, n := range notes {
		noteIDs[i] = n.ID
	}
	s.store.MarkNotesConsolidated(noteIDs)

	s.store.UpdateConsolidationLog(
		cl.ID,
		model.ConsolidationCompleted,
		result.Summary,
		strings.Join(appliedFiles, ","),
	)

	return &SleepResult{
		LogID:          cl.ID,
		NotesProcessed: len(notes),
		Summary:        result.Summary,
		KBFilesUpdated: appliedFiles,
		TasksCreated:   len(result.SuggestedTasks),
	}, nil
}

// Apply writes approved KB files and creates approved tasks.
func (s *Service) Apply(ctx context.Context, changes *ProposedChanges, approvedKBIndices []int, approvedTaskIndices []int) error {
	var appliedFiles []string

	// Write approved KB files
	for _, idx := range approvedKBIndices {
		if idx < 0 || idx >= len(changes.KBUpdates) {
			continue
		}
		u := changes.KBUpdates[idx]
		if err := s.kb.Write(u.Path, u.Content); err != nil {
			return fmt.Errorf("write KB file %s: %w", u.Path, err)
		}
		appliedFiles = append(appliedFiles, u.Path)
	}

	// Create approved tasks
	for _, idx := range approvedTaskIndices {
		if idx < 0 || idx >= len(changes.SuggestedTasks) {
			continue
		}
		_, err := s.store.CreateTask(changes.SuggestedTasks[idx], "", nil, 0)
		if err != nil {
			return fmt.Errorf("create task: %w", err)
		}
	}

	// Record consolidation event
	payload, _ := json.Marshal(map[string]any{
		"log_id":       changes.LogID,
		"kb_files":     appliedFiles,
		"tasks_created": len(approvedTaskIndices),
	})
	s.store.RecordConsolidationEvent(changes.SessionID, string(payload))

	// Update log to completed
	s.store.UpdateConsolidationLog(
		changes.LogID,
		model.ConsolidationCompleted,
		changes.Summary,
		strings.Join(appliedFiles, ","),
	)

	return nil
}
