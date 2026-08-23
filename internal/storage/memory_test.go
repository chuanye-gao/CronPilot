package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

func TestMemoryTaskLifecycleAndExecutionOrder(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	created, err := store.CreateTask(ctx, task.Task{Name: "one"})
	if err != nil || created.ID == "" {
		t.Fatalf("CreateTask() = %#v, %v", created, err)
	}
	created.Name = "updated"
	updated, err := store.UpdateTask(ctx, created)
	if err != nil || updated.Name != "updated" {
		t.Fatalf("UpdateTask() = %#v, %v", updated, err)
	}
	first := execution.Execution{ID: "run_1", TaskID: created.ID}
	second := execution.Execution{ID: "run_2", TaskID: created.ID}
	_ = store.CreateExecution(ctx, first)
	_ = store.CreateExecution(ctx, second)
	runs, _ := store.ListExecutions(ctx, created.ID, 10)
	if len(runs) != 2 || runs[0].ID != "run_2" {
		t.Fatalf("runs = %#v", runs)
	}
	if err := store.DeleteTask(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}
	if _, err := store.GetTask(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() error = %v", err)
	}
}
