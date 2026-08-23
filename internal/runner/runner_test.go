package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type fakeClient struct {
	complete func(context.Context, string) (string, error)
}

type fakeDelivery struct {
	called atomic.Bool
	err    error
}

func (f *fakeDelivery) Deliver(context.Context, task.Task, execution.Execution) error {
	f.called.Store(true)
	return f.err
}

func (f fakeClient) Complete(ctx context.Context, prompt string) (string, error) {
	return f.complete(ctx, prompt)
}

func TestRunTaskSuccess(t *testing.T) {
	runner, _ := newTestRunner(func(_ context.Context, prompt string) (string, error) {
		return "result: " + prompt, nil
	})
	run, err := runner.RunTask(context.Background(), testTask())
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if run.Status != execution.StatusSuccess || run.Output != "result: do work" || run.Attempts != 1 {
		t.Fatalf("execution = %#v", run)
	}
}

func TestRunTaskRetriesFailures(t *testing.T) {
	var attempts atomic.Int32
	runner, _ := newTestRunner(func(_ context.Context, _ string) (string, error) {
		if attempts.Add(1) < 3 {
			return "", errors.New("temporary error")
		}
		return "recovered", nil
	})
	value := testTask()
	value.Retry.MaxAttempts = 3
	value.Retry.Delay = task.Duration(time.Millisecond)
	run, err := runner.RunTask(context.Background(), value)
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if run.Status != execution.StatusSuccess || run.Attempts != 3 || run.Output != "recovered" {
		t.Fatalf("execution = %#v", run)
	}
}

func TestRunTaskTimeout(t *testing.T) {
	runner, _ := newTestRunner(func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	value := testTask()
	value.Timeout = task.Duration(10 * time.Millisecond)
	run, err := runner.RunTask(context.Background(), value)
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if run.Status != execution.StatusTimeout || run.FinishedAt == nil {
		t.Fatalf("execution = %#v", run)
	}
}

func TestDeliveryFailureDoesNotChangeExecutionSuccess(t *testing.T) {
	store := storage.NewMemory()
	delivery := &fakeDelivery{err: errors.New("smtp unavailable")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := New(fakeClient{complete: func(context.Context, string) (string, error) { return "done", nil }}, store, logger, WithDelivery(delivery))
	value := testTask()
	value.Delivery = task.Delivery{Type: "email", To: []string{"owner@example.com"}, On: []string{"success"}}
	run, err := runner.RunTask(context.Background(), value)
	if err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if run.Status != execution.StatusSuccess || run.DeliveryStatus != "failed" || !delivery.called.Load() {
		t.Fatalf("execution = %#v", run)
	}
}

func TestShutdownInterruptsRunningTasks(t *testing.T) {
	started := make(chan struct{})
	store := storage.NewMemory()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := New(fakeClient{complete: func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}, store, logger)
	run, err := runner.StartTask(context.Background(), testTask())
	if err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	<-started
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runner.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	finished, err := store.GetExecution(context.Background(), run.ID)
	if err != nil || finished.Status != execution.StatusInterrupted || finished.FinishedAt == nil {
		t.Fatalf("interrupted execution = %#v, %v", finished, err)
	}
	if _, err := runner.StartTask(context.Background(), testTask()); err == nil {
		t.Fatal("StartTask() succeeded after shutdown")
	}
}

func TestShutdownInterruptsSynchronousScheduledTask(t *testing.T) {
	started := make(chan struct{})
	runner, _ := newTestRunner(func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	result := make(chan execution.Execution, 1)
	go func() {
		run, _ := runner.RunTask(context.Background(), testTask())
		result <- run
	}()
	<-started
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runner.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if run := <-result; run.Status != execution.StatusInterrupted {
		t.Fatalf("execution status = %q", run.Status)
	}
}

func newTestRunner(complete func(context.Context, string) (string, error)) (*Runner, *storage.Memory) {
	store := storage.NewMemory()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(fakeClient{complete: complete}, store, logger), store
}

func testTask() task.Task {
	enabled := true
	return task.Task{
		ID: "task_test", Name: "test", Schedule: "* * * * *", Prompt: "do work", Enabled: &enabled,
		Timeout: task.Duration(time.Second), Retry: task.Retry{MaxAttempts: 1, Delay: task.Duration(time.Millisecond)},
	}
}
