package config

import "testing"

func TestLoadRuntimeDefaults(t *testing.T) {
	t.Setenv("SB_SLEEP_THRESHOLD", "")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "")
	t.Setenv("SB_TASK_PRIORITY_MAX", "")
	t.Setenv("SB_SYNC_FOCUS_NOTES_LIMIT", "")
	t.Setenv("SB_SYNC_FOCUS_TASKS_LIMIT", "")
	t.Setenv("SB_SYNC_FOCUS_TAGS_MAX", "")
	t.Setenv("SB_SYNC_FOCUS_TERMS_MAX", "")
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "")
	t.Setenv("SB_FEATURE_SYNC_FOCUS_LEARNING", "")
	t.Setenv("SB_METRICS_WINDOW_DAYS", "")

	cfg := LoadRuntime()
	if cfg.SleepThreshold != 10 {
		t.Fatalf("unexpected default sleep threshold: %d", cfg.SleepThreshold)
	}
	if cfg.SyncPredictionWindow != 5 {
		t.Fatalf("unexpected default prediction window: %d", cfg.SyncPredictionWindow)
	}
	if cfg.PriorityAdjustLimit != 5 {
		t.Fatalf("unexpected default priority adjust limit: %d", cfg.PriorityAdjustLimit)
	}
	if cfg.SleepReplayAlpha != 0.18 {
		t.Fatalf("unexpected default sleep replay alpha: %f", cfg.SleepReplayAlpha)
	}
	if cfg.SleepDuplicateReplayWeight != 0.35 {
		t.Fatalf("unexpected default duplicate replay weight: %f", cfg.SleepDuplicateReplayWeight)
	}
	if cfg.TaskPriorityMax != 5 {
		t.Fatalf("unexpected default task priority max: %d", cfg.TaskPriorityMax)
	}
	if cfg.SyncFocusNotesLimit != 250 {
		t.Fatalf("unexpected default sync focus notes limit: %d", cfg.SyncFocusNotesLimit)
	}
	if cfg.SyncFocusTasksLimit != 120 {
		t.Fatalf("unexpected default sync focus tasks limit: %d", cfg.SyncFocusTasksLimit)
	}
	if cfg.SyncFocusTagsMax != 8 {
		t.Fatalf("unexpected default sync focus tags max: %d", cfg.SyncFocusTagsMax)
	}
	if cfg.SyncFocusTermsMax != 12 {
		t.Fatalf("unexpected default sync focus terms max: %d", cfg.SyncFocusTermsMax)
	}
	if !cfg.PredictionLearningEnabled {
		t.Fatal("prediction learning should default to enabled")
	}
	if !cfg.SleepReplayEnabled {
		t.Fatal("sleep replay should default to enabled")
	}
	if !cfg.SyncFocusLearningEnabled {
		t.Fatal("sync focus learning should default to enabled")
	}
	if cfg.MetricsWindowDays != 14 {
		t.Fatalf("unexpected default metrics window: %d", cfg.MetricsWindowDays)
	}
}

func TestLoadRuntimeOverridesAndBounds(t *testing.T) {
	t.Setenv("SB_SLEEP_THRESHOLD", "25")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "9")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "3")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "0.2")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "0.4")
	t.Setenv("SB_TASK_PRIORITY_MAX", "9")
	t.Setenv("SB_SYNC_FOCUS_NOTES_LIMIT", "180")
	t.Setenv("SB_SYNC_FOCUS_TASKS_LIMIT", "90")
	t.Setenv("SB_SYNC_FOCUS_TAGS_MAX", "6")
	t.Setenv("SB_SYNC_FOCUS_TERMS_MAX", "16")
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "false")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "0")
	t.Setenv("SB_FEATURE_SYNC_FOCUS_LEARNING", "off")
	t.Setenv("SB_METRICS_WINDOW_DAYS", "30")

	cfg := LoadRuntime()
	if cfg.SleepThreshold != 25 ||
		cfg.SyncPredictionWindow != 9 ||
		cfg.PriorityAdjustLimit != 3 ||
		cfg.SleepReplayAlpha != 0.2 ||
		cfg.SleepDuplicateReplayWeight != 0.4 ||
		cfg.TaskPriorityMax != 9 ||
		cfg.SyncFocusNotesLimit != 180 ||
		cfg.SyncFocusTasksLimit != 90 ||
		cfg.SyncFocusTagsMax != 6 ||
		cfg.SyncFocusTermsMax != 16 ||
		cfg.PredictionLearningEnabled ||
		cfg.SleepReplayEnabled ||
		cfg.SyncFocusLearningEnabled ||
		cfg.MetricsWindowDays != 30 {
		t.Fatalf("unexpected overridden config: %+v", cfg)
	}
}
