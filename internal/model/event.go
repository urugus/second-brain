package model

import "time"

type EventType string

const (
	EventSessionStarted    EventType = "session.started"
	EventSessionEnded      EventType = "session.ended"
	EventSessionAbandoned  EventType = "session.abandoned"
	EventTaskCreated       EventType = "task.created"
	EventTaskStatusChanged EventType = "task.status_changed"
	EventNoteAdded         EventType = "note.added"
	EventConsolidated      EventType = "session.consolidated"
)

type Event struct {
	ID        int64
	SessionID *int64
	Type      EventType
	Payload   string // JSON
	CreatedAt time.Time
}
