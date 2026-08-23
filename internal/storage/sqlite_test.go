package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

func TestSQLitePersistsTasksAndExecutions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "cronpilot.db")
	store, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite() error = %v", err)
	}
	ctx := context.Background()
	enabled := true
	includeOutput := false
	created, err := store.CreateTask(ctx, task.Task{
		Name: "Daily brief", Description: "Persistent task", Schedule: "0 8 * * *", Timezone: "Asia/Shanghai",
		Prompt: "Write a brief", Enabled: &enabled, Timeout: task.Duration(time.Minute),
		Retry:    task.Retry{MaxAttempts: 2, Delay: task.Duration(time.Second)},
		Delivery: task.Delivery{Type: "email", To: []string{"owner@example.com"}, On: []string{"success"}, IncludeOutput: &includeOutput},
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	run := execution.New(created)
	if err := store.CreateExecution(ctx, run); err != nil {
		t.Fatalf("CreateExecution() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("reopen SQLite error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	found, err := reopened.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if found.Name != created.Name || found.Delivery.To[0] != "owner@example.com" || found.Delivery.IncludesOutput() {
		t.Fatalf("persisted task = %#v", found)
	}
	foundRun, err := reopened.GetExecution(ctx, run.ID)
	if err != nil || foundRun.TaskID != created.ID || foundRun.Status != execution.StatusPending {
		t.Fatalf("persisted execution = %#v, %v", foundRun, err)
	}
}

func TestSQLiteRecoversInterruptedExecutions(t *testing.T) {
	store, err := NewSQLite(filepath.Join(t.TempDir(), "cronpilot.db"))
	if err != nil {
		t.Fatalf("NewSQLite() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	created, err := store.CreateTask(ctx, task.Task{Name: "test"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	pending := execution.New(created)
	if err := store.CreateExecution(ctx, pending); err != nil {
		t.Fatalf("CreateExecution() error = %v", err)
	}

	recoveredAt := time.Now().UTC().Truncate(time.Millisecond)
	count, err := store.RecoverInterruptedExecutions(ctx, recoveredAt)
	if err != nil || count != 1 {
		t.Fatalf("RecoverInterruptedExecutions() = %d, %v", count, err)
	}
	found, err := store.GetExecution(ctx, pending.ID)
	if err != nil || found.Status != execution.StatusInterrupted || found.FinishedAt == nil || found.Error == "" {
		t.Fatalf("recovered execution = %#v, %v", found, err)
	}
	if err := store.DeleteTask(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}
	if _, err := store.GetTask(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() error = %v", err)
	}
	if _, err := store.GetExecution(ctx, pending.ID); err != nil {
		t.Fatalf("execution history was deleted with task: %v", err)
	}
}

func TestSQLiteListsUseEmptyArrays(t *testing.T) {
	store, err := NewSQLite(filepath.Join(t.TempDir(), "cronpilot.db"))
	if err != nil {
		t.Fatalf("NewSQLite() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tasks, err := store.ListTasks(context.Background())
	if err != nil || tasks == nil || len(tasks) != 0 {
		t.Fatalf("ListTasks() = %#v, %v", tasks, err)
	}
	runs, err := store.ListExecutions(context.Background(), "", 50)
	if err != nil || runs == nil || len(runs) != 0 {
		t.Fatalf("ListExecutions() = %#v, %v", runs, err)
	}
}
