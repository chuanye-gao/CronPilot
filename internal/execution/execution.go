package execution

import (
	"time"

	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type Status string

const (
	StatusPending     Status = "pending"
	StatusRunning     Status = "running"
	StatusSuccess     Status = "success"
	StatusFailed      Status = "failed"
	StatusTimeout     Status = "timeout"
	StatusInterrupted Status = "interrupted"
)

type Execution struct {
	ID             string     `json:"id"`
	OwnerID        string     `json:"-"`
	TaskID         string     `json:"task_id"`
	TaskName       string     `json:"task_name"`
	Status         Status     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Output         string     `json:"output,omitempty"`
	Error          string     `json:"error,omitempty"`
	Attempts       int        `json:"attempts"`
	DeliveryStatus string     `json:"delivery_status,omitempty"`
	DeliveryError  string     `json:"delivery_error,omitempty"`
}

func New(t task.Task) Execution {
	return Execution{
		ID:        id.New("run"),
		OwnerID:   t.OwnerID,
		TaskID:    t.ID,
		TaskName:  t.Name,
		Status:    StatusPending,
		StartedAt: time.Now().UTC(),
	}
}

func (e Execution) Duration() time.Duration {
	if e.FinishedAt == nil {
		return time.Since(e.StartedAt)
	}
	return e.FinishedAt.Sub(e.StartedAt)
}
