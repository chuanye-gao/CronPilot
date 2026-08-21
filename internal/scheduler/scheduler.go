package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/runner"
	"github.com/chuanye-gao/CronPilot/internal/task"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron   *cron.Cron
	runner *runner.Runner
	logger *slog.Logger
}

func New(location *time.Location, r *runner.Runner, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(cron.WithLocation(location)),
		runner: r,
		logger: logger,
	}
}

func (s *Scheduler) Add(t task.Task) error {
	if !t.IsEnabled() {
		s.logger.Info("task disabled", "task", t.Name)
		return nil
	}

	_, err := s.cron.AddFunc(t.Schedule, func() {
		s.runner.Run(context.Background(), t)
	})
	if err != nil {
		return fmt.Errorf("schedule task %q: %w", t.Name, err)
	}

	s.logger.Info("task scheduled", "task", t.Name, "schedule", t.Schedule)
	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() context.Context {
	return s.cron.Stop()
}
