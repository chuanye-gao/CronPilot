package delivery

import (
	"context"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type Delivery interface {
	Deliver(context.Context, task.Task, execution.Execution) error
}
