package config

import "testing"

func TestLoadRuntimeDefaults(t *testing.T) {
	t.Setenv("SB_SLEEP_THRESHOLD", "")
	t.Setenv("SB_SLEEP_POLICY_SCORE_THRESHOLD", "")
	t.Setenv("SB_SLEEP_POLICY_RECURRENCE_WEIGHT", "")
	t.Setenv("SB_SLEEP_POLICY_UTILITY_WEIGHT", "")
	t.Setenv("SB_SLEEP_POLICY_STALENESS_WEIGHT", "")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_WEIGHT", "")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_MAX_PAIRS", "")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_WEIGHT", "")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE", "")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_CANDIDATES", "")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS", "")
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_ALPHA", "")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_DECAY", "")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_MAX_EDGES", "")
	t.Setenv("SB_ENTITY_AUTOEDGE_MAX_PAIRS", "")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_WEIGHT", "")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MAX_LINKS", "")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MIN_SHARED", "")
	t.Setenv("SB_ENTITY_FEEDBACK_ALPHA", "")
	t.Setenv("SB_ENTITY_FEEDBACK_DECAY", "")
	t.Setenv("SB_ENTITY_FEEDBACK_MAX_ENTITIES", "")
	t.Setenv("SB_ENTITY_DECAY_RATE", "")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "")
	t.Setenv("SB_TASK_PRIORITY_MAX", "")
	t.Setenv("SB_SYNC_FOCUS_NOTES_LIMIT", "")
	t.Setenv("SB_SYNC_FOCUS_TASKS_LIMIT", "")
	t.Setenv("SB_SYNC_FOCUS_TAGS_MAX", "")
	t.Setenv("SB_SYNC_FOCUS_TERMS_MAX", "")
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "")
	t.Setenv("SB_FEATURE_SYNC_FOCUS_LEARNING", "")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_AUTOLINK", "")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK", "")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_DECAY", "")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_FEEDBACK", "")
	t.Setenv("SB_FEATURE_ENTITY_LEARNING", "")
	t.Setenv("SB_FEATURE_ENTITY_DERIVED_EDGE", "")
	t.Setenv("SB_FEATURE_ENTITY_FEEDBACK", "")
	t.Setenv("SB_FEATURE_ENTITY_DECAY", "")
	t.Setenv("SB_METRICS_WINDOW_DAYS", "")

	cfg := LoadRuntime()
	if cfg.SleepThreshold != 10 {
		t.Fatalf("unexpected default sleep threshold: %d", cfg.SleepThreshold)
	}
	if cfg.SleepPolicyScoreThreshold != 0.20 {
		t.Fatalf("unexpected default sleep policy score threshold: %f", cfg.SleepPolicyScoreThreshold)
	}
	if cfg.SleepPolicyRecurrenceW != 0.35 {
		t.Fatalf("unexpected default sleep policy recurrence weight: %f", cfg.SleepPolicyRecurrenceW)
	}
	if cfg.SleepPolicyUtilityW != 0.55 {
		t.Fatalf("unexpected default sleep policy utility weight: %f", cfg.SleepPolicyUtilityW)
	}
	if cfg.SleepPolicyStalenessW != 0.25 {
		t.Fatalf("unexpected default sleep policy staleness weight: %f", cfg.SleepPolicyStalenessW)
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
	if cfg.MemoryEdgeAutoLinkWeight != 0.12 {
		t.Fatalf("unexpected default memory edge autolink weight: %f", cfg.MemoryEdgeAutoLinkWeight)
	}
	if cfg.MemoryEdgeAutoLinkMaxPairs != 24 {
		t.Fatalf("unexpected default memory edge autolink max pairs: %d", cfg.MemoryEdgeAutoLinkMaxPairs)
	}
	if cfg.MemoryEdgeCreateWeight != 0.20 {
		t.Fatalf("unexpected default create autolink weight: %f", cfg.MemoryEdgeCreateWeight)
	}
	if cfg.MemoryEdgeCreateMinScore != 0.34 {
		t.Fatalf("unexpected default create autolink min score: %f", cfg.MemoryEdgeCreateMinScore)
	}
	if cfg.MemoryEdgeCreateCandidates != 80 {
		t.Fatalf("unexpected default create autolink candidates: %d", cfg.MemoryEdgeCreateCandidates)
	}
	if cfg.MemoryEdgeCreateMaxLinks != 3 {
		t.Fatalf("unexpected default create autolink max links: %d", cfg.MemoryEdgeCreateMaxLinks)
	}
	if cfg.MemoryEdgeDecayRate != 0.010 {
		t.Fatalf("unexpected default memory edge decay rate: %f", cfg.MemoryEdgeDecayRate)
	}
	if cfg.MemoryEdgeMinWeight != 0.02 {
		t.Fatalf("unexpected default memory edge min weight: %f", cfg.MemoryEdgeMinWeight)
	}
	if cfg.MemoryEdgeFeedbackAlpha != 0.12 {
		t.Fatalf("unexpected default memory edge feedback alpha: %f", cfg.MemoryEdgeFeedbackAlpha)
	}
	if cfg.MemoryEdgeFeedbackDecay != 0.05 {
		t.Fatalf("unexpected default memory edge feedback decay: %f", cfg.MemoryEdgeFeedbackDecay)
	}
	if cfg.MemoryEdgeFeedbackMaxEdges != 10 {
		t.Fatalf("unexpected default memory edge feedback max edges: %d", cfg.MemoryEdgeFeedbackMaxEdges)
	}
	if cfg.EntityAutoEdgeMaxPairs != 20 {
		t.Fatalf("unexpected default entity autoedge max pairs: %d", cfg.EntityAutoEdgeMaxPairs)
	}
	if cfg.EntityDerivedEdgeWeight != 0.14 {
		t.Fatalf("unexpected default entity derived edge weight: %f", cfg.EntityDerivedEdgeWeight)
	}
	if cfg.EntityDerivedEdgeMaxLinks != 4 {
		t.Fatalf("unexpected default entity derived edge max links: %d", cfg.EntityDerivedEdgeMaxLinks)
	}
	if cfg.EntityDerivedEdgeMinShared != 1 {
		t.Fatalf("unexpected default entity derived edge min shared: %d", cfg.EntityDerivedEdgeMinShared)
	}
	if cfg.EntityFeedbackAlpha != 0.10 {
		t.Fatalf("unexpected default entity feedback alpha: %f", cfg.EntityFeedbackAlpha)
	}
	if cfg.EntityFeedbackDecay != 0.04 {
		t.Fatalf("unexpected default entity feedback decay: %f", cfg.EntityFeedbackDecay)
	}
	if cfg.EntityFeedbackMaxEntities != 10 {
		t.Fatalf("unexpected default entity feedback max entities: %d", cfg.EntityFeedbackMaxEntities)
	}
	if cfg.EntityDecayRate != 0.008 {
		t.Fatalf("unexpected default entity decay rate: %f", cfg.EntityDecayRate)
	}
	if cfg.EntityMinStrength != 0.10 {
		t.Fatalf("unexpected default entity min strength: %f", cfg.EntityMinStrength)
	}
	if cfg.EntityMinSalience != 0.20 {
		t.Fatalf("unexpected default entity min salience: %f", cfg.EntityMinSalience)
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
	if !cfg.MemoryEdgeAutoLinkEnabled {
		t.Fatal("memory edge autolink should default to enabled")
	}
	if cfg.MemoryEdgeCreateEnabled {
		t.Fatal("memory edge create autolink should default to disabled")
	}
	if !cfg.MemoryEdgeDecayEnabled {
		t.Fatal("memory edge decay should default to enabled")
	}
	if !cfg.MemoryEdgeFeedbackEnabled {
		t.Fatal("memory edge feedback should default to enabled")
	}
	if !cfg.EntityLearningEnabled {
		t.Fatal("entity learning should default to enabled")
	}
	if !cfg.EntityDerivedEdgeEnabled {
		t.Fatal("entity derived edge should default to enabled")
	}
	if !cfg.EntityFeedbackEnabled {
		t.Fatal("entity feedback should default to enabled")
	}
	if !cfg.EntityDecayEnabled {
		t.Fatal("entity decay should default to enabled")
	}
	if cfg.MetricsWindowDays != 14 {
		t.Fatalf("unexpected default metrics window: %d", cfg.MetricsWindowDays)
	}
}

func TestLoadRuntimeOverridesAndBounds(t *testing.T) {
	t.Setenv("SB_SLEEP_THRESHOLD", "25")
	t.Setenv("SB_SLEEP_POLICY_SCORE_THRESHOLD", "0.42")
	t.Setenv("SB_SLEEP_POLICY_RECURRENCE_WEIGHT", "0.30")
	t.Setenv("SB_SLEEP_POLICY_UTILITY_WEIGHT", "0.60")
	t.Setenv("SB_SLEEP_POLICY_STALENESS_WEIGHT", "0.20")
	t.Setenv("SB_SYNC_PREDICTION_WINDOW", "9")
	t.Setenv("SB_PRIORITY_ADJUST_LIMIT", "3")
	t.Setenv("SB_SLEEP_REPLAY_ALPHA", "0.2")
	t.Setenv("SB_SLEEP_DUPLICATE_REPLAY_WEIGHT", "0.4")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_WEIGHT", "0.19")
	t.Setenv("SB_MEMORY_EDGE_AUTOLINK_MAX_PAIRS", "8")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_WEIGHT", "0.23")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE", "0.41")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_CANDIDATES", "120")
	t.Setenv("SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS", "5")
	t.Setenv("SB_MEMORY_EDGE_DECAY_RATE", "0.03")
	t.Setenv("SB_MEMORY_EDGE_MIN_WEIGHT", "0.05")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_ALPHA", "0.22")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_DECAY", "0.11")
	t.Setenv("SB_MEMORY_EDGE_FEEDBACK_MAX_EDGES", "7")
	t.Setenv("SB_ENTITY_AUTOEDGE_MAX_PAIRS", "15")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_WEIGHT", "0.18")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MAX_LINKS", "6")
	t.Setenv("SB_ENTITY_DERIVED_EDGE_MIN_SHARED", "2")
	t.Setenv("SB_ENTITY_FEEDBACK_ALPHA", "0.16")
	t.Setenv("SB_ENTITY_FEEDBACK_DECAY", "0.08")
	t.Setenv("SB_ENTITY_FEEDBACK_MAX_ENTITIES", "6")
	t.Setenv("SB_ENTITY_DECAY_RATE", "0.02")
	t.Setenv("SB_ENTITY_MIN_STRENGTH", "0.15")
	t.Setenv("SB_ENTITY_MIN_SALIENCE", "0.30")
	t.Setenv("SB_TASK_PRIORITY_MAX", "9")
	t.Setenv("SB_SYNC_FOCUS_NOTES_LIMIT", "180")
	t.Setenv("SB_SYNC_FOCUS_TASKS_LIMIT", "90")
	t.Setenv("SB_SYNC_FOCUS_TAGS_MAX", "6")
	t.Setenv("SB_SYNC_FOCUS_TERMS_MAX", "16")
	t.Setenv("SB_FEATURE_PREDICTION_LEARNING", "false")
	t.Setenv("SB_FEATURE_SLEEP_REPLAY", "0")
	t.Setenv("SB_FEATURE_SYNC_FOCUS_LEARNING", "off")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_AUTOLINK", "false")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK", "1")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_DECAY", "0")
	t.Setenv("SB_FEATURE_MEMORY_EDGE_FEEDBACK", "false")
	t.Setenv("SB_FEATURE_ENTITY_LEARNING", "off")
	t.Setenv("SB_FEATURE_ENTITY_DERIVED_EDGE", "0")
	t.Setenv("SB_FEATURE_ENTITY_FEEDBACK", "false")
	t.Setenv("SB_FEATURE_ENTITY_DECAY", "0")
	t.Setenv("SB_METRICS_WINDOW_DAYS", "30")

	cfg := LoadRuntime()
	if cfg.SleepThreshold != 25 ||
		cfg.SleepPolicyScoreThreshold != 0.42 ||
		cfg.SleepPolicyRecurrenceW != 0.30 ||
		cfg.SleepPolicyUtilityW != 0.60 ||
		cfg.SleepPolicyStalenessW != 0.20 ||
		cfg.SyncPredictionWindow != 9 ||
		cfg.PriorityAdjustLimit != 3 ||
		cfg.SleepReplayAlpha != 0.2 ||
		cfg.SleepDuplicateReplayWeight != 0.4 ||
		cfg.MemoryEdgeAutoLinkWeight != 0.19 ||
		cfg.MemoryEdgeAutoLinkMaxPairs != 8 ||
		cfg.MemoryEdgeCreateWeight != 0.23 ||
		cfg.MemoryEdgeCreateMinScore != 0.41 ||
		cfg.MemoryEdgeCreateCandidates != 120 ||
		cfg.MemoryEdgeCreateMaxLinks != 5 ||
		cfg.MemoryEdgeDecayRate != 0.03 ||
		cfg.MemoryEdgeMinWeight != 0.05 ||
		cfg.MemoryEdgeFeedbackAlpha != 0.22 ||
		cfg.MemoryEdgeFeedbackDecay != 0.11 ||
		cfg.MemoryEdgeFeedbackMaxEdges != 7 ||
		cfg.EntityAutoEdgeMaxPairs != 15 ||
		cfg.EntityDerivedEdgeWeight != 0.18 ||
		cfg.EntityDerivedEdgeMaxLinks != 6 ||
		cfg.EntityDerivedEdgeMinShared != 2 ||
		cfg.EntityFeedbackAlpha != 0.16 ||
		cfg.EntityFeedbackDecay != 0.08 ||
		cfg.EntityFeedbackMaxEntities != 6 ||
		cfg.EntityDecayRate != 0.02 ||
		cfg.EntityMinStrength != 0.15 ||
		cfg.EntityMinSalience != 0.30 ||
		cfg.TaskPriorityMax != 9 ||
		cfg.SyncFocusNotesLimit != 180 ||
		cfg.SyncFocusTasksLimit != 90 ||
		cfg.SyncFocusTagsMax != 6 ||
		cfg.SyncFocusTermsMax != 16 ||
		cfg.PredictionLearningEnabled ||
		cfg.SleepReplayEnabled ||
		cfg.SyncFocusLearningEnabled ||
		cfg.MemoryEdgeAutoLinkEnabled ||
		!cfg.MemoryEdgeCreateEnabled ||
		cfg.MemoryEdgeDecayEnabled ||
		cfg.MemoryEdgeFeedbackEnabled ||
		cfg.EntityLearningEnabled ||
		cfg.EntityDerivedEdgeEnabled ||
		cfg.EntityFeedbackEnabled ||
		cfg.EntityDecayEnabled ||
		cfg.MetricsWindowDays != 30 {
		t.Fatalf("unexpected overridden config: %+v", cfg)
	}
}
