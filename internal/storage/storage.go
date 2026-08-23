package storage

import (
	"context"
	"errors"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	Ping(context.Context) error
	ListTasks(context.Context) ([]task.Task, error)
	GetTask(context.Context, string) (task.Task, error)
	CreateTask(context.Context, task.Task) (task.Task, error)
	UpdateTask(context.Context, task.Task) (task.Task, error)
	DeleteTask(context.Context, string) error
	ListExecutions(context.Context, string, int) ([]execution.Execution, error)
	ListExecutionsByOwner(context.Context, string, int) ([]execution.Execution, error)
	GetExecution(context.Context, string) (execution.Execution, error)
	CreateExecution(context.Context, execution.Execution) error
	UpdateExecution(context.Context, execution.Execution) error
}

type RecoveryStore interface {
	RecoverInterruptedExecutions(context.Context, time.Time) (int64, error)
}
