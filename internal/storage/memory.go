package storage

import (
	"context"
	"sync"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type Memory struct {
	mu             sync.RWMutex
	tasks          map[string]task.Task
	taskOrder      []string
	executions     map[string]execution.Execution
	executionOrder []string
}

func NewMemory() *Memory {
	return &Memory{tasks: make(map[string]task.Task), executions: make(map[string]execution.Execution)}
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) RecoverInterruptedExecutions(_ context.Context, recoveredAt time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var recovered int64
	for executionID, value := range m.executions {
		if value.Status != execution.StatusPending && value.Status != execution.StatusRunning {
			continue
		}
		finishedAt := recoveredAt.UTC()
		value.Status = execution.StatusInterrupted
		value.FinishedAt = &finishedAt
		value.Error = "CronPilot stopped before this execution finished"
		m.executions[executionID] = value
		recovered++
	}
	return recovered, nil
}

func (m *Memory) ListTasks(_ context.Context) ([]task.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]task.Task, 0, len(m.taskOrder))
	for _, taskID := range m.taskOrder {
		result = append(result, m.tasks[taskID])
	}
	return result, nil
}

func (m *Memory) GetTask(_ context.Context, taskID string) (task.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	found, ok := m.tasks[taskID]
	if !ok {
		return task.Task{}, ErrNotFound
	}
	return found, nil
}

func (m *Memory) CreateTask(_ context.Context, value task.Task) (task.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value.ID == "" {
		value.ID = id.New("task")
	}
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	m.tasks[value.ID] = value
	m.taskOrder = append(m.taskOrder, value.ID)
	return value, nil
}

func (m *Memory) UpdateTask(_ context.Context, value task.Task) (task.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tasks[value.ID]
	if !ok {
		return task.Task{}, ErrNotFound
	}
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.tasks[value.ID] = value
	return value, nil
}

func (m *Memory) DeleteTask(_ context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[taskID]; !ok {
		return ErrNotFound
	}
	delete(m.tasks, taskID)
	for i, idValue := range m.taskOrder {
		if idValue == taskID {
			m.taskOrder = append(m.taskOrder[:i], m.taskOrder[i+1:]...)
			break
		}
	}
	return nil
}

func (m *Memory) ListExecutions(_ context.Context, taskID string, limit int) ([]execution.Execution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]execution.Execution, 0, limit)
	for i := len(m.executionOrder) - 1; i >= 0 && len(result) < limit; i-- {
		found := m.executions[m.executionOrder[i]]
		if taskID == "" || found.TaskID == taskID {
			result = append(result, found)
		}
	}
	return result, nil
}

func (m *Memory) ListExecutionsByOwner(_ context.Context, ownerID string, limit int) ([]execution.Execution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]execution.Execution, 0, limit)
	for i := len(m.executionOrder) - 1; i >= 0 && len(result) < limit; i-- {
		found := m.executions[m.executionOrder[i]]
		if found.OwnerID == ownerID {
			result = append(result, found)
		}
	}
	return result, nil
}

func (m *Memory) GetExecution(_ context.Context, executionID string) (execution.Execution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	found, ok := m.executions[executionID]
	if !ok {
		return execution.Execution{}, ErrNotFound
	}
	return found, nil
}

func (m *Memory) CreateExecution(_ context.Context, value execution.Execution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions[value.ID] = value
	m.executionOrder = append(m.executionOrder, value.ID)
	return nil
}

func (m *Memory) UpdateExecution(_ context.Context, value execution.Execution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.executions[value.ID]; !ok {
		return ErrNotFound
	}
	m.executions[value.ID] = value
	return nil
}
