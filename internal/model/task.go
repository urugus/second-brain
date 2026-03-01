package model

import "time"

type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID          int64
	SessionID   *int64
	Title       string
	Description string
	Status      TaskStatus
	Priority    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
