package sync

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/urugus/second-brain/internal/config"
	"github.com/urugus/second-brain/internal/model"
)

var syncFocusTokenPattern = regexp.MustCompile(`[\p{L}\p{N}#@_./:+-]+`)

var syncFocusStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "by": {},
	"for": {}, "from": {}, "in": {}, "is": {}, "it": {}, "of": {}, "on": {}, "or": {},
	"that": {}, "the": {}, "this": {}, "to": {}, "we": {}, "with": {}, "you": {},
}

type focusProfile struct {
	Tags      []string
	Terms     []string
	TaskTerms []string
}

func (p *focusProfile) isEmpty() bool {
	return p == nil || (len(p.Tags) == 0 && len(p.Terms) == 0 && len(p.TaskTerms) == 0)
}

func (s *Service) buildSyncFocusProfile(runtimeCfg config.Runtime) *focusProfile {
	if !runtimeCfg.SyncFocusLearningEnabled {
		return nil
	}

	notes, err := s.store.ListRecentNotesForSyncFocus(runtimeCfg.SyncFocusNotesLimit)
	if err != nil {
		notes = nil
	}

	tasks, err := s.store.ListActiveTasksForSyncFocus(runtimeCfg.SyncFocusTasksLimit)
	if err != nil {
		tasks = nil
	}

	if len(notes) == 0 && len(tasks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	tagScores := map[string]float64{}
	noteTermScores := map[string]float64{}
	taskTermScores := map[string]float64{}

	for _, n := range notes {
		addNoteLearningSignal(now, n, tagScores, noteTermScores)
	}
	for _, t := range tasks {
		addTaskLearningSignal(now, t, taskTermScores)
	}

	profile := &focusProfile{
		Tags:      topScoredKeys(tagScores, runtimeCfg.SyncFocusTagsMax),
		Terms:     topScoredKeys(noteTermScores, runtimeCfg.SyncFocusTermsMax),
		TaskTerms: topScoredKeys(taskTermScores, runtimeCfg.SyncFocusTermsMax),
	}
	if profile.isEmpty() {
		return nil
	}
	return profile
}

func addNoteLearningSignal(now time.Time, note model.Note, tagScores, termScores map[string]float64) {
	base := noteLearningWeight(now, note)

	for _, tag := range note.Tags {
		normTag := normalizeLearningTerm(tag)
		if normTag == "" {
			continue
		}
		tagScores[normTag] += base * 1.8
	}

	terms := tokenizeLearningTerms(note.Content)
	if len(terms) == 0 {
		return
	}
	perTerm := base / float64(len(terms))
	for _, term := range terms {
		termScores[term] += perTerm
	}
}

func addTaskLearningSignal(now time.Time, task model.Task, taskTermScores map[string]float64) {
	base := taskLearningWeight(now, task)
	terms := tokenizeLearningTerms(task.Title + " " + task.Description)
	if len(terms) == 0 {
		return
	}
	perTerm := base / float64(len(terms))
	for _, term := range terms {
		taskTermScores[term] += perTerm
	}
}

func noteLearningWeight(now time.Time, note model.Note) float64 {
	ageHours := now.Sub(note.UpdatedAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	recency := 1.0 / (1.0 + ageHours/168.0)

	sourceBoost := 1.0
	switch strings.ToLower(strings.TrimSpace(note.Source)) {
	case "manual", "human":
		sourceBoost = 1.35
	case "sync", "claude-code":
		sourceBoost = 1.0
	default:
		sourceBoost = 1.1
	}

	memoryBoost := 0.8 + (0.6 * clampFloat(note.Salience, 0, 1)) + (0.6 * clampFloat(note.Strength, 0, 1))
	recallBoost := 1.0 + clampFloat(float64(note.RecallCount)*0.08, 0, 0.8)
	return recency * sourceBoost * memoryBoost * recallBoost
}

func taskLearningWeight(now time.Time, task model.Task) float64 {
	ageHours := now.Sub(task.UpdatedAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	recency := 1.0 / (1.0 + ageHours/96.0)

	statusBoost := 1.0
	if task.Status == model.TaskInProgress {
		statusBoost = 1.3
	}

	priority := task.Priority
	if priority < 0 {
		priority = 0
	}
	if priority > 10 {
		priority = 10
	}
	priorityBoost := 1.0 + (0.35 * float64(priority))

	return recency * statusBoost * priorityBoost
}

func tokenizeLearningTerms(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	matches := syncFocusTokenPattern.FindAllString(strings.ToLower(text), -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	terms := make([]string, 0, len(matches))
	for _, raw := range matches {
		term := normalizeLearningTerm(raw)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}

func normalizeLearningTerm(raw string) string {
	term := strings.ToLower(strings.TrimSpace(raw))
	term = strings.Trim(term, ".,;:!?\"'`()[]{}<>|\\")
	term = strings.Trim(term, "、。！？「」『』（）［］【】")
	if term == "" {
		return ""
	}

	if strings.HasPrefix(term, "http://") || strings.HasPrefix(term, "https://") {
		return ""
	}
	if len(term) < 2 || len(term) > 48 {
		return ""
	}
	if _, ok := syncFocusStopwords[term]; ok {
		return ""
	}
	if isNumericLike(term) {
		return ""
	}
	return term
}

func isNumericLike(v string) bool {
	for _, r := range v {
		if (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == '/' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func topScoredKeys(scores map[string]float64, maxCount int) []string {
	if maxCount <= 0 || len(scores) == 0 {
		return nil
	}

	type scored struct {
		Key   string
		Score float64
	}

	ranked := make([]scored, 0, len(scores))
	for key, score := range scores {
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scored{Key: key, Score: score})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Key < ranked[j].Key
		}
		return ranked[i].Score > ranked[j].Score
	})

	if len(ranked) > maxCount {
		ranked = ranked[:maxCount]
	}

	keys := make([]string, 0, len(ranked))
	for _, item := range ranked {
		keys = append(keys, item.Key)
	}
	return keys
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
