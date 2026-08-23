package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
	"github.com/robfig/cron/v3"
)

type TaskRunner interface {
	RunTask(context.Context, task.Task) (execution.Execution, error)
}

type Scheduler struct {
	cron    *cron.Cron
	runner  TaskRunner
	logger  *slog.Logger
	mu      sync.RWMutex
	entries map[string]cron.EntryID
	ready   atomic.Bool
}

func New(location *time.Location, runner TaskRunner, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:    cron.New(cron.WithLocation(location)),
		runner:  runner,
		logger:  logger,
		entries: make(map[string]cron.EntryID),
	}
}

func (s *Scheduler) Add(value task.Task) error {
	return s.Upsert(value)
}

func (s *Scheduler) Upsert(value task.Task) error {
	s.Remove(value.ID)
	if !value.IsEnabled() {
		s.logger.Info("task disabled", "task", value.Name)
		return nil
	}

	schedule := value.Schedule
	if value.Timezone != "" {
		schedule = "CRON_TZ=" + value.Timezone + " " + schedule
	}
	entryID, err := s.cron.AddFunc(schedule, func() {
		if _, runErr := s.runner.RunTask(context.Background(), value); runErr != nil {
			s.logger.Error("run scheduled task", "task", value.Name, "error", runErr)
		}
	})
	if err != nil {
		return fmt.Errorf("schedule task %q: %w", value.Name, err)
	}

	s.mu.Lock()
	s.entries[value.ID] = entryID
	s.mu.Unlock()
	s.logger.Info("task scheduled", "task", value.Name, "schedule", value.Schedule, "timezone", value.Timezone)
	return nil
}

func (s *Scheduler) Remove(taskID string) {
	s.mu.Lock()
	entryID, ok := s.entries[taskID]
	if ok {
		delete(s.entries, taskID)
	}
	s.mu.Unlock()
	if ok {
		s.cron.Remove(entryID)
	}
}

func (s *Scheduler) NextRun(taskID string) time.Time {
	s.mu.RLock()
	entryID, ok := s.entries[taskID]
	s.mu.RUnlock()
	if !ok {
		return time.Time{}
	}
	return s.cron.Entry(entryID).Next
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.ready.Store(true)
}

func (s *Scheduler) Stop() context.Context {
	s.ready.Store(false)
	return s.cron.Stop()
}

func (s *Scheduler) Ready() bool {
	return s.ready.Load()
}
