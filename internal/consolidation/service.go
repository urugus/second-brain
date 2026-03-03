package consolidation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/adapter"
	"github.com/urugus/second-brain/internal/config"
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
	LogID            int64
	NotesProcessed   int
	PolicyCandidates int
	PolicySelected   int
	PolicyThreshold  float64
	PolicyReasons    []string
	NotesReplayed    int
	DuplicatesMerged int
	Summary          string
	KBFilesUpdated   []string
	TasksCreated     int
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
	runtimeCfg := config.LoadRuntime()
	now := time.Now().UTC()
	policyResult := applySleepLongTermPolicy(notes, now, runtimeCfg)
	if len(policyResult.SelectedNotes) == 0 {
		return nil, nil
	}
	if len(policyResult.SelectedNotes) < threshold {
		return nil, nil
	}
	replayPlan := buildSleepReplayPlan(policyResult.SelectedNotes, runtimeCfg.SleepDuplicateReplayWeight, runtimeCfg.SleepReplayEnabled)
	if len(replayPlan.replayNotes) == 0 {
		return nil, nil
	}
	predictedKBUpdates := estimateSleepKBUpdates(len(replayPlan.replayNotes))

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
		Notes:      replayPlan.replayNotes,
		ExistingKB: kbFiles,
	}

	result, err := s.agent.Consolidate(ctx, req)
	if err != nil {
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, fmt.Sprintf("agent error: %v", err), "")
		return nil, fmt.Errorf("agent consolidation: %w", err)
	}
	dedupedKBUpdates := dedupeKBUpdatesByPath(result.KBUpdates)

	var appliedFiles []string
	var writeErrors []string
	autoLinkedPairs := make(map[string]struct{})
	for _, u := range dedupedKBUpdates {
		content := stripRelatedSection(u.Content)
		if err := s.kb.Write(u.Path, content); err != nil {
			writeErrors = append(writeErrors, fmt.Sprintf("%s: %v", u.Path, err))
			continue
		}
		appliedFiles = append(appliedFiles, u.Path)

		// Record note→KB mapping
		noteIDs := selectRelevantNoteIDsForKBUpdate(u.Content, replayPlan.replayNotes)
		if len(noteIDs) > 0 {
			_ = s.store.MapKBNotes(u.Path, noteIDs)
			s.autoLinkNotesForKBUpdate(noteIDs, u.Path, runtimeCfg, autoLinkedPairs)
			for _, note := range selectNotesByIDs(noteIDs, replayPlan.replayNotes) {
				_ = s.store.LearnEntitiesFromNote(note, "sleep_consolidation")
			}
		}
	}

	// Append Related sections (after all mappings are recorded)
	for _, path := range appliedFiles {
		section := buildRelatedSection(path, s.store, s.kb)
		if section == "" {
			continue
		}
		existing, err := s.kb.Read(path)
		if err != nil {
			continue
		}
		_ = s.kb.Write(path, existing+section)
	}

	if len(writeErrors) > 0 {
		errMsg := fmt.Sprintf(
			"kb writes failed (%d/%d): %s",
			len(writeErrors),
			len(dedupedKBUpdates),
			strings.Join(writeErrors, "; "),
		)
		if runtimeCfg.PredictionLearningEnabled {
			s.recordSleepPredictionError(predictedKBUpdates, float64(len(appliedFiles)))
		}
		s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, errMsg, strings.Join(appliedFiles, ","))
		return nil, fmt.Errorf("%s", errMsg)
	}

	tasksCreated := 0
	for _, taskTitle := range dedupeStrings(result.SuggestedTasks) {
		if _, err := s.store.CreateTask(taskTitle, "", nil, 0); err == nil {
			tasksCreated++
		}
	}

	if runtimeCfg.SleepReplayEnabled {
		if err := s.store.ApplySleepReplayConsolidation(replayPlan.replayWeightByNoteID, time.Now().UTC()); err != nil {
			errMsg := fmt.Sprintf("update note consolidation state: %v", err)
			s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, errMsg, strings.Join(appliedFiles, ","))
			return nil, fmt.Errorf("%s", errMsg)
		}
	} else {
		noteIDs := make([]int64, len(policyResult.SelectedNotes))
		for i, n := range policyResult.SelectedNotes {
			noteIDs[i] = n.ID
		}
		if err := s.store.MarkNotesConsolidated(noteIDs); err != nil {
			errMsg := fmt.Sprintf("mark notes consolidated: %v", err)
			s.store.UpdateConsolidationLog(cl.ID, model.ConsolidationFailed, errMsg, strings.Join(appliedFiles, ","))
			return nil, fmt.Errorf("%s", errMsg)
		}
	}

	s.store.UpdateConsolidationLog(
		cl.ID,
		model.ConsolidationCompleted,
		result.Summary,
		strings.Join(appliedFiles, ","),
	)
	if runtimeCfg.PredictionLearningEnabled {
		s.recordSleepPredictionError(predictedKBUpdates, float64(len(appliedFiles)))
	}

	return &SleepResult{
		LogID:            cl.ID,
		NotesProcessed:   len(notes),
		PolicyCandidates: policyResult.CandidateCount,
		PolicySelected:   len(policyResult.SelectedNotes),
		PolicyThreshold:  policyResult.Threshold,
		PolicyReasons:    summarizeSleepPolicyReasons(policyResult.Decisions, 5),
		NotesReplayed:    len(replayPlan.replayNotes),
		DuplicatesMerged: replayPlan.duplicatesMerged,
		Summary:          result.Summary,
		KBFilesUpdated:   appliedFiles,
		TasksCreated:     tasksCreated,
	}, nil
}

type sleepReplayPlan struct {
	replayNotes          []model.Note
	replayWeightByNoteID map[int64]float64
	duplicatesMerged     int
}

func buildSleepReplayPlan(notes []model.Note, duplicateWeight float64, replayEnabled bool) sleepReplayPlan {
	if len(notes) == 0 {
		return sleepReplayPlan{
			replayWeightByNoteID: map[int64]float64{},
		}
	}
	if !replayEnabled {
		replayWeightByNoteID := make(map[int64]float64, len(notes))
		for _, n := range notes {
			replayWeightByNoteID[n.ID] = 1.0
		}
		return sleepReplayPlan{
			replayNotes:          notes,
			replayWeightByNoteID: replayWeightByNoteID,
			duplicatesMerged:     0,
		}
	}

	type bucket struct {
		canonical model.Note
	}

	buckets := map[string]*bucket{}
	var order []string
	replayWeightByNoteID := make(map[int64]float64, len(notes))
	duplicatesMerged := 0

	for _, n := range notes {
		key := normalizeNoteContent(n.Content)
		if key == "" {
			key = fmt.Sprintf("__note_id__:%d", n.ID)
		}

		b, ok := buckets[key]
		if !ok {
			buckets[key] = &bucket{canonical: n}
			order = append(order, key)
			replayWeightByNoteID[n.ID] = 1.0
			continue
		}

		current := b.canonical
		if shouldReplaceCanonical(current, n) {
			replayWeightByNoteID[current.ID] = duplicateWeight
			b.canonical = n
			replayWeightByNoteID[n.ID] = 1.0
		} else {
			replayWeightByNoteID[n.ID] = duplicateWeight
		}
		duplicatesMerged++
	}

	replayNotes := make([]model.Note, 0, len(order))
	for _, key := range order {
		replayNotes = append(replayNotes, buckets[key].canonical)
	}
	sort.SliceStable(replayNotes, func(i, j int) bool {
		return replayPriority(replayNotes[i]) > replayPriority(replayNotes[j])
	})

	return sleepReplayPlan{
		replayNotes:          replayNotes,
		replayWeightByNoteID: replayWeightByNoteID,
		duplicatesMerged:     duplicatesMerged,
	}
}

func shouldReplaceCanonical(current model.Note, candidate model.Note) bool {
	currentScore := replayPriority(current)
	candidateScore := replayPriority(candidate)
	if candidateScore == currentScore {
		return candidate.ID < current.ID
	}
	return candidateScore > currentScore
}

func replayPriority(note model.Note) float64 {
	return (note.Salience * 0.6) + (note.Strength * 0.4)
}

func normalizeNoteContent(content string) string {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return ""
	}
	return strings.Join(strings.Fields(normalized), " ")
}

func dedupeKBUpdatesByPath(updates []adapter.KBUpdate) []adapter.KBUpdate {
	if len(updates) == 0 {
		return nil
	}
	indices := make(map[string]int, len(updates))
	out := make([]adapter.KBUpdate, 0, len(updates))
	for _, u := range updates {
		if idx, ok := indices[u.Path]; ok {
			out[idx] = u
			continue
		}
		indices[u.Path] = len(out)
		out = append(out, u)
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func selectRelevantNoteIDsForKBUpdate(kbContent string, notes []model.Note) []int64 {
	if len(notes) == 0 {
		return nil
	}
	if len(notes) == 1 {
		return []int64{notes[0].ID}
	}

	normalizedKB := normalizeNoteContent(kbContent)
	if normalizedKB == "" {
		return nil
	}

	seen := make(map[int64]struct{}, len(notes))
	selected := make([]int64, 0, len(notes))
	for _, note := range notes {
		noteKey := normalizeNoteContent(note.Content)
		if noteKey != "" && strings.Contains(normalizedKB, noteKey) {
			if _, ok := seen[note.ID]; !ok {
				seen[note.ID] = struct{}{}
				selected = append(selected, note.ID)
			}
			continue
		}

		for _, tag := range note.Tags {
			tagKey := normalizeNoteContent(tag)
			if tagKey == "" || !strings.Contains(normalizedKB, tagKey) {
				continue
			}
			if _, ok := seen[note.ID]; ok {
				break
			}
			seen[note.ID] = struct{}{}
			selected = append(selected, note.ID)
			break
		}
	}

	return selected
}

func (s *Service) autoLinkNotesForKBUpdate(noteIDs []int64, kbPath string, cfg config.Runtime, linkedPairs map[string]struct{}) {
	if !cfg.MemoryEdgeAutoLinkEnabled || cfg.MemoryEdgeAutoLinkWeight <= 0 || cfg.MemoryEdgeAutoLinkMaxPairs <= 0 {
		return
	}

	uniqueNoteIDs := uniqueSortedNoteIDs(noteIDs)
	if len(uniqueNoteIDs) < 2 {
		return
	}

	evidence := fmt.Sprintf("auto:kb-cooccurrence:%s", kbPath)
	for i := 0; i < len(uniqueNoteIDs); i++ {
		if len(linkedPairs) >= cfg.MemoryEdgeAutoLinkMaxPairs {
			break
		}
		for j := i + 1; j < len(uniqueNoteIDs); j++ {
			if len(linkedPairs) >= cfg.MemoryEdgeAutoLinkMaxPairs {
				break
			}
			a := uniqueNoteIDs[i]
			b := uniqueNoteIDs[j]
			key := notePairKey(a, b)
			if _, exists := linkedPairs[key]; exists {
				continue
			}

			if err := s.store.LinkNotes(a, b, cfg.MemoryEdgeAutoLinkWeight, evidence); err != nil {
				continue
			}
			if err := s.store.LinkNotes(b, a, cfg.MemoryEdgeAutoLinkWeight, evidence); err != nil {
				continue
			}

			linkedPairs[key] = struct{}{}
		}
	}
}

func uniqueSortedNoteIDs(noteIDs []int64) []int64 {
	if len(noteIDs) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(noteIDs))
	out := make([]int64, 0, len(noteIDs))
	for _, id := range noteIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func notePairKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d:%d", a, b)
}

func selectNotesByIDs(noteIDs []int64, notes []model.Note) []model.Note {
	if len(noteIDs) == 0 || len(notes) == 0 {
		return nil
	}

	index := make(map[int64]model.Note, len(notes))
	for _, note := range notes {
		index[note.ID] = note
	}

	selected := make([]model.Note, 0, len(noteIDs))
	seen := make(map[int64]struct{}, len(noteIDs))
	for _, id := range noteIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		note, ok := index[id]
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, note)
	}
	return selected
}

func summarizeSleepPolicyReasons(decisions []sleepPolicyDecision, limit int) []string {
	if len(decisions) == 0 || limit <= 0 {
		return nil
	}
	if len(decisions) < limit {
		limit = len(decisions)
	}
	reasons := make([]string, 0, limit)
	for _, d := range decisions[:limit] {
		reasons = append(reasons, fmt.Sprintf("note#%d %s", d.NoteID, d.Reason))
	}
	return reasons
}

func estimateSleepKBUpdates(replayedNotes int) float64 {
	if replayedNotes <= 0 {
		return 0
	}
	estimate := float64(replayedNotes) / 3.0
	if estimate < 1 {
		return 1
	}
	return estimate
}

func (s *Service) recordSleepPredictionError(predicted, actual float64) {
	_ = s.store.RecordPredictionError(
		model.PredictionSourceSleep,
		"kb_updates",
		predicted,
		actual,
		0,
	)
}

// Apply writes approved KB files and creates approved tasks.
func (s *Service) Apply(ctx context.Context, changes *ProposedChanges, approvedKBIndices []int, approvedTaskIndices []int) error {
	var appliedFiles []string

	// Gather session notes for selective KB mapping
	sessionNotes, _ := s.store.ListNotes(store.NoteFilter{SessionID: &changes.SessionID})
	runtimeCfg := config.LoadRuntime()
	autoLinkedPairs := make(map[string]struct{})

	// Write approved KB files
	for _, idx := range approvedKBIndices {
		if idx < 0 || idx >= len(changes.KBUpdates) {
			continue
		}
		u := changes.KBUpdates[idx]
		content := stripRelatedSection(u.Content)
		if err := s.kb.Write(u.Path, content); err != nil {
			return fmt.Errorf("write KB file %s: %w", u.Path, err)
		}
		appliedFiles = append(appliedFiles, u.Path)

		// Record note→KB mapping
		noteIDs := selectRelevantNoteIDsForKBUpdate(u.Content, sessionNotes)
		if len(noteIDs) > 0 {
			_ = s.store.MapKBNotes(u.Path, noteIDs)
			s.autoLinkNotesForKBUpdate(noteIDs, u.Path, runtimeCfg, autoLinkedPairs)
			for _, note := range selectNotesByIDs(noteIDs, sessionNotes) {
				_ = s.store.LearnEntitiesFromNote(note, "consolidation_apply")
			}
		}
	}

	// Append Related sections (after all mappings are recorded)
	for _, path := range appliedFiles {
		section := buildRelatedSection(path, s.store, s.kb)
		if section == "" {
			continue
		}
		existing, err := s.kb.Read(path)
		if err != nil {
			continue
		}
		_ = s.kb.Write(path, existing+section)
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
		"log_id":        changes.LogID,
		"kb_files":      appliedFiles,
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
