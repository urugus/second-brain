package model

type OperationalMetrics struct {
	WindowDays               int
	NotesTotal               int
	DuplicateNotes           int
	DuplicateNoteRate        float64
	TasksTotal               int
	TasksDone                int
	UsefulTaskGenerationRate float64
	UniqueKBFilesUpdated     int
	ReworkedKBFiles          int
	KBReworkRate             float64
}
