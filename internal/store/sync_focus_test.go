package store

import (
	"testing"

	"github.com/urugus/second-brain/internal/model"
)

func TestListActiveTasksForSyncFocus_PrioritizesInProgressBeforeTodo(t *testing.T) {
	s := setupTestStore(t)

	todo, err := s.CreateTask("backlog", "high priority backlog", nil, 5)
	if err != nil {
		t.Fatalf("create todo task: %v", err)
	}
	inProgress, err := s.CreateTask("active", "currently doing", nil, 0)
	if err != nil {
		t.Fatalf("create in-progress task: %v", err)
	}
	if err := s.UpdateTaskStatus(inProgress.ID, model.TaskInProgress); err != nil {
		t.Fatalf("set in_progress: %v", err)
	}

	got, err := s.ListActiveTasksForSyncFocus(1)
	if err != nil {
		t.Fatalf("list active tasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].ID != inProgress.ID {
		t.Fatalf("expected in-progress task %d first, got %d (todo task was %d)", inProgress.ID, got[0].ID, todo.ID)
	}
}

func TestListActiveTasksForSyncFocus_ExcludesDoneAndCancelled(t *testing.T) {
	s := setupTestStore(t)

	todo, err := s.CreateTask("todo", "", nil, 1)
	if err != nil {
		t.Fatalf("create todo task: %v", err)
	}
	inProgress, err := s.CreateTask("in-progress", "", nil, 1)
	if err != nil {
		t.Fatalf("create in-progress task: %v", err)
	}
	if err := s.UpdateTaskStatus(inProgress.ID, model.TaskInProgress); err != nil {
		t.Fatalf("set in_progress: %v", err)
	}
	done, err := s.CreateTask("done", "", nil, 1)
	if err != nil {
		t.Fatalf("create done task: %v", err)
	}
	if err := s.UpdateTaskStatus(done.ID, model.TaskDone); err != nil {
		t.Fatalf("set done: %v", err)
	}
	cancelled, err := s.CreateTask("cancelled", "", nil, 1)
	if err != nil {
		t.Fatalf("create cancelled task: %v", err)
	}
	if err := s.UpdateTaskStatus(cancelled.ID, model.TaskCancelled); err != nil {
		t.Fatalf("set cancelled: %v", err)
	}

	got, err := s.ListActiveTasksForSyncFocus(10)
	if err != nil {
		t.Fatalf("list active tasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 active tasks, got %d", len(got))
	}

	idSet := map[int64]struct{}{}
	for _, tsk := range got {
		if tsk.Status != model.TaskTodo && tsk.Status != model.TaskInProgress {
			t.Fatalf("unexpected status in active tasks: %s", tsk.Status)
		}
		idSet[tsk.ID] = struct{}{}
	}
	if _, ok := idSet[todo.ID]; !ok {
		t.Fatalf("expected todo task %d in result", todo.ID)
	}
	if _, ok := idSet[inProgress.ID]; !ok {
		t.Fatalf("expected in-progress task %d in result", inProgress.ID)
	}
}
