package model

import "time"

type ConsolidationStatus string

const (
	ConsolidationPending   ConsolidationStatus = "pending"
	ConsolidationRunning   ConsolidationStatus = "running"
	ConsolidationCompleted ConsolidationStatus = "completed"
	ConsolidationFailed    ConsolidationStatus = "failed"
)

type ConsolidationLog struct {
	ID             int64
	SessionID      int64
	Agent          string
	InputSummary   string
	OutputSummary  string
	KBFilesUpdated string // comma-separated paths
	Status         ConsolidationStatus
	CreatedAt      time.Time
}
