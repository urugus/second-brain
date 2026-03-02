package config

import (
	"os"
	"strconv"
	"strings"
)

type Runtime struct {
	SleepThreshold             int
	SleepPolicyScoreThreshold  float64
	SleepPolicyRecurrenceW     float64
	SleepPolicyUtilityW        float64
	SleepPolicyStalenessW      float64
	SyncPredictionWindow       int
	PriorityAdjustLimit        int
	SleepReplayAlpha           float64
	SleepDuplicateReplayWeight float64
	TaskPriorityMax            int
	SyncFocusNotesLimit        int
	SyncFocusTasksLimit        int
	SyncFocusTagsMax           int
	SyncFocusTermsMax          int
	PredictionLearningEnabled  bool
	SleepReplayEnabled         bool
	SyncFocusLearningEnabled   bool
	MetricsWindowDays          int
}

func LoadRuntime() Runtime {
	return Runtime{
		SleepThreshold:             getInt("SB_SLEEP_THRESHOLD", 10, 1, 10_000),
		SleepPolicyScoreThreshold:  getFloat("SB_SLEEP_POLICY_SCORE_THRESHOLD", 0.20, 0.0, 1.0),
		SleepPolicyRecurrenceW:     getFloat("SB_SLEEP_POLICY_RECURRENCE_WEIGHT", 0.35, 0.0, 1.0),
		SleepPolicyUtilityW:        getFloat("SB_SLEEP_POLICY_UTILITY_WEIGHT", 0.55, 0.0, 1.0),
		SleepPolicyStalenessW:      getFloat("SB_SLEEP_POLICY_STALENESS_WEIGHT", 0.25, 0.0, 1.0),
		SyncPredictionWindow:       getInt("SB_SYNC_PREDICTION_WINDOW", 5, 1, 100),
		PriorityAdjustLimit:        getInt("SB_PRIORITY_ADJUST_LIMIT", 5, 1, 100),
		SleepReplayAlpha:           getFloat("SB_SLEEP_REPLAY_ALPHA", 0.18, 0.0, 1.0),
		SleepDuplicateReplayWeight: getFloat("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", 0.35, 0.0, 1.0),
		TaskPriorityMax:            getInt("SB_TASK_PRIORITY_MAX", 5, 1, 100),
		SyncFocusNotesLimit:        getInt("SB_SYNC_FOCUS_NOTES_LIMIT", 250, 10, 5000),
		SyncFocusTasksLimit:        getInt("SB_SYNC_FOCUS_TASKS_LIMIT", 120, 5, 2000),
		SyncFocusTagsMax:           getInt("SB_SYNC_FOCUS_TAGS_MAX", 8, 1, 50),
		SyncFocusTermsMax:          getInt("SB_SYNC_FOCUS_TERMS_MAX", 12, 1, 100),
		PredictionLearningEnabled:  getBool("SB_FEATURE_PREDICTION_LEARNING", true),
		SleepReplayEnabled:         getBool("SB_FEATURE_SLEEP_REPLAY", true),
		SyncFocusLearningEnabled:   getBool("SB_FEATURE_SYNC_FOCUS_LEARNING", true),
		MetricsWindowDays:          getInt("SB_METRICS_WINDOW_DAYS", 14, 1, 365),
	}
}

func getInt(name string, def int, minV int, maxV int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func getFloat(name string, def float64, minV float64, maxV float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func getBool(name string, def bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
