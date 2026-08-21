package task

import "fmt"

type Task struct {
	Name     string `yaml:"name"`
	Schedule string `yaml:"schedule"`
	Prompt   string `yaml:"prompt"`
	Enabled  *bool  `yaml:"enabled,omitempty"`
}

func (t Task) IsEnabled() bool {
	return t.Enabled == nil || *t.Enabled
}

func (t Task) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if t.Schedule == "" {
		return fmt.Errorf("task %q: schedule is required", t.Name)
	}
	if t.Prompt == "" {
		return fmt.Errorf("task %q: prompt is required", t.Name)
	}
	return nil
}
