package model

import "time"

type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionAbandoned SessionStatus = "abandoned"
)

type Session struct {
	ID        int64
	Title     string
	Goal      string
	Status    SessionStatus
	StartedAt time.Time
	EndedAt   *time.Time
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}
