package model

import "time"

type MemoryEdge struct {
	ID              int64
	FromNoteID      int64
	ToNoteID        int64
	Weight          float64
	Evidence        string
	ReinforcedCount int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RelatedNote struct {
	Note  Note
	Score float64
}
