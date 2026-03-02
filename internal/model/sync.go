package model

import "time"

type SyncStatus string

const (
	SyncPending   SyncStatus = "pending"
	SyncRunning   SyncStatus = "running"
	SyncCompleted SyncStatus = "completed"
	SyncFailed    SyncStatus = "failed"
)

type SyncLog struct {
	ID             int64
	Agent          string
	PromptUsed     string
	OutputSummary  string
	NotesAdded     int
	TasksAdded     int
	KBFilesUpdated string // comma-separated paths
	DurationMs     int64
	Status         SyncStatus
	ErrorMessage   string
	CreatedAt      time.Time
}
