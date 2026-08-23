package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/llm"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type ExecutionStore interface {
	CreateExecution(context.Context, execution.Execution) error
	UpdateExecution(context.Context, execution.Execution) error
	GetExecution(context.Context, string) (execution.Execution, error)
}

type Runner struct {
	client   llm.Client
	store    ExecutionStore
	delivery Delivery
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closing  bool
	wg       sync.WaitGroup
}

type Delivery interface {
	Deliver(context.Context, task.Task, execution.Execution) error
}

type Option func(*Runner)

func WithDelivery(value Delivery) Option {
	return func(runner *Runner) { runner.delivery = value }
}

func New(client llm.Client, store ExecutionStore, logger *slog.Logger, options ...Option) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	result := &Runner{client: client, store: store, logger: logger, ctx: ctx, cancel: cancel}
	for _, option := range options {
		option(result)
	}
	return result
}

func (r *Runner) StartTask(ctx context.Context, value task.Task) (execution.Execution, error) {
	if err := r.start(); err != nil {
		return execution.Execution{}, err
	}
	run := execution.New(value)
	if err := r.store.CreateExecution(ctx, run); err != nil {
		r.wg.Done()
		return execution.Execution{}, fmt.Errorf("create execution for task %q: %w", value.Name, err)
	}
	go func() {
		defer r.wg.Done()
		r.execute(r.ctx, value, run)
	}()
	return run, nil
}

func (r *Runner) RunTask(ctx context.Context, value task.Task) (execution.Execution, error) {
	if err := r.start(); err != nil {
		return execution.Execution{}, err
	}
	defer r.wg.Done()
	run := execution.New(value)
	if err := r.store.CreateExecution(ctx, run); err != nil {
		return execution.Execution{}, fmt.Errorf("create execution for task %q: %w", value.Name, err)
	}
	executionCtx, cancel := r.executionContext(ctx)
	r.execute(executionCtx, value, run)
	cancel()
	updated, err := r.current(ctx, run)
	if err != nil {
		return run, err
	}
	return updated, nil
}

func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if !r.closing {
		r.closing = true
		r.cancel()
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for task executions: %w", ctx.Err())
	}
}

func (r *Runner) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return fmt.Errorf("cronpilot is shutting down")
	}
	r.wg.Add(1)
	return nil
}

func (r *Runner) executionContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(r.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (r *Runner) execute(parent context.Context, value task.Task, run execution.Execution) {
	run.Status = execution.StatusRunning
	run.StartedAt = time.Now().UTC()
	if err := r.store.UpdateExecution(parent, run); err != nil {
		r.logger.Error("mark execution running", "task", value.Name, "execution", run.ID, "error", err)
		return
	}

	timeout := time.Duration(value.Timeout)
	if timeout <= 0 {
		timeout = task.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	maxAttempts := value.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	delay := time.Duration(value.Retry.Delay)
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		run.Attempts = attempt
		r.logger.Info("task attempt started", "task", value.Name, "execution", run.ID, "attempt", attempt)
		output, err := r.client.Complete(ctx, value.Prompt)
		if err == nil {
			run.Output = output
			run.Status = execution.StatusSuccess
			lastErr = nil
			break
		}
		lastErr = err
		if ctx.Err() != nil || attempt == maxAttempts {
			break
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	if ctx.Err() != nil {
		lastErr = ctx.Err()
	}

	finished := time.Now().UTC()
	run.FinishedAt = &finished
	if lastErr != nil {
		run.Error = lastErr.Error()
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			run.Status = execution.StatusTimeout
		case errors.Is(ctx.Err(), context.Canceled):
			run.Status = execution.StatusInterrupted
		case run.Status != execution.StatusSuccess:
			run.Status = execution.StatusFailed
		}
	}

	if err := r.store.UpdateExecution(context.Background(), run); err != nil {
		r.logger.Error("finish execution", "task", value.Name, "execution", run.ID, "error", err)
		return
	}
	r.deliver(value, &run)
	if run.Status == execution.StatusSuccess {
		r.logger.Info("task completed", "task", value.Name, "execution", run.ID, "attempts", run.Attempts, "duration", run.Duration())
		return
	}
	r.logger.Error("task failed", "task", value.Name, "execution", run.ID, "status", run.Status, "attempts", run.Attempts, "error", run.Error, "duration", run.Duration())
}

func (r *Runner) deliver(value task.Task, run *execution.Execution) {
	if !value.Delivery.ShouldNotify(string(run.Status)) {
		return
	}
	run.DeliveryStatus = "pending"
	if err := r.store.UpdateExecution(context.Background(), *run); err != nil {
		r.logger.Error("mark delivery pending", "task", value.Name, "execution", run.ID, "error", err)
		return
	}
	if r.delivery == nil {
		run.DeliveryStatus = "failed"
		run.DeliveryError = "email delivery is not configured"
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := r.delivery.Deliver(ctx, value, *run)
		cancel()
		if err != nil {
			run.DeliveryStatus = "failed"
			run.DeliveryError = err.Error()
		} else {
			run.DeliveryStatus = "sent"
			run.DeliveryError = ""
		}
	}
	if err := r.store.UpdateExecution(context.Background(), *run); err != nil {
		r.logger.Error("finish delivery", "task", value.Name, "execution", run.ID, "error", err)
		return
	}
	if run.DeliveryStatus == "failed" {
		r.logger.Error("email delivery failed", "task", value.Name, "execution", run.ID, "error", run.DeliveryError)
		return
	}
	r.logger.Info("email delivered", "task", value.Name, "execution", run.ID, "recipients", len(value.Delivery.To))
}

func (r *Runner) current(ctx context.Context, fallback execution.Execution) (execution.Execution, error) {
	current, err := r.store.GetExecution(ctx, fallback.ID)
	if err != nil {
		return fallback, fmt.Errorf("read execution %q: %w", fallback.ID, err)
	}
	return current, nil
}
