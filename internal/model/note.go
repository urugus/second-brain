package model

import "time"

type Note struct {
	ID             int64
	SessionID      *int64
	Content        string
	Tags           []string
	Source         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ConsolidatedAt *time.Time
}
