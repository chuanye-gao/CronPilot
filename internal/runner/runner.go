package runner

import (
	"context"
	"log/slog"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/llm"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type Runner struct {
	client llm.Client
	logger *slog.Logger
}

func New(client llm.Client, logger *slog.Logger) *Runner {
	return &Runner{client: client, logger: logger}
}

func (r *Runner) Run(ctx context.Context, t task.Task) {
	started := time.Now()
	r.logger.Info("task started", "task", t.Name)

	result, err := r.client.Complete(ctx, t.Prompt)
	if err != nil {
		r.logger.Error("task failed", "task", t.Name, "error", err, "duration", time.Since(started))
		return
	}

	r.logger.Info("task completed", "task", t.Name, "duration", time.Since(started), "result", result)
}
