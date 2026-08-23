package task

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const DefaultTimeout = 5 * time.Minute

type Duration time.Duration

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	return d.parse(value)
}

func (d *Duration) UnmarshalText(text []byte) error {
	return d.parse(string(text))
}

func (d *Duration) parse(value string) error {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", value, err)
	}
	*d = Duration(parsed)
	return nil
}

type Retry struct {
	MaxAttempts int      `yaml:"max_attempts,omitempty" json:"max_attempts"`
	Delay       Duration `yaml:"delay,omitempty" json:"delay"`
}

type Delivery struct {
	Type          string   `yaml:"type,omitempty" json:"type,omitempty"`
	To            []string `yaml:"to,omitempty" json:"to,omitempty"`
	On            []string `yaml:"on,omitempty" json:"on,omitempty"`
	IncludeOutput *bool    `yaml:"include_output,omitempty" json:"include_output,omitempty"`
}

type Task struct {
	ID          string    `yaml:"id,omitempty" json:"id"`
	OwnerID     string    `yaml:"-" json:"-"`
	Name        string    `yaml:"name" json:"name"`
	Description string    `yaml:"description,omitempty" json:"description"`
	Schedule    string    `yaml:"schedule" json:"schedule"`
	Timezone    string    `yaml:"timezone,omitempty" json:"timezone"`
	Prompt      string    `yaml:"prompt" json:"prompt"`
	Enabled     *bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Timeout     Duration  `yaml:"timeout,omitempty" json:"timeout"`
	Retry       Retry     `yaml:"retry,omitempty" json:"retry"`
	Delivery    Delivery  `yaml:"delivery,omitempty" json:"delivery,omitempty"`
	CreatedAt   time.Time `yaml:"created_at,omitempty" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at,omitempty" json:"updated_at"`
}

func (t Task) IsEnabled() bool {
	return t.Enabled == nil || *t.Enabled
}

func (t *Task) ApplyDefaults(defaultTimezone string) {
	t.Name = strings.TrimSpace(t.Name)
	t.Description = strings.TrimSpace(t.Description)
	t.Schedule = strings.TrimSpace(t.Schedule)
	t.Prompt = strings.TrimSpace(t.Prompt)
	if t.Timezone == "" {
		t.Timezone = defaultTimezone
	}
	if t.Timeout == 0 {
		t.Timeout = Duration(DefaultTimeout)
	}
	if t.Retry.MaxAttempts == 0 {
		t.Retry.MaxAttempts = 1
	}
	if t.Retry.Delay == 0 {
		t.Retry.Delay = Duration(10 * time.Second)
	}
	t.Delivery.Type = strings.ToLower(strings.TrimSpace(t.Delivery.Type))
	for i := range t.Delivery.To {
		t.Delivery.To[i] = strings.TrimSpace(t.Delivery.To[i])
	}
	if t.Delivery.Type == "email" && len(t.Delivery.On) == 0 {
		t.Delivery.On = []string{"success", "failed", "timeout"}
	}
}

func (t Task) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if t.Schedule == "" {
		return fmt.Errorf("task %q: schedule is required", t.Name)
	}
	if _, err := cron.ParseStandard(t.Schedule); err != nil {
		return fmt.Errorf("task %q: invalid schedule: %w", t.Name, err)
	}
	if t.Timezone != "" {
		if _, err := time.LoadLocation(t.Timezone); err != nil {
			return fmt.Errorf("task %q: invalid timezone %q: %w", t.Name, t.Timezone, err)
		}
	}
	if t.Prompt == "" {
		return fmt.Errorf("task %q: prompt is required", t.Name)
	}
	if time.Duration(t.Timeout) <= 0 {
		return fmt.Errorf("task %q: timeout must be positive", t.Name)
	}
	if t.Retry.MaxAttempts < 1 || t.Retry.MaxAttempts > 10 {
		return fmt.Errorf("task %q: retry.max_attempts must be between 1 and 10", t.Name)
	}
	if time.Duration(t.Retry.Delay) < 0 {
		return fmt.Errorf("task %q: retry.delay cannot be negative", t.Name)
	}
	if err := t.Delivery.Validate(t.Name); err != nil {
		return err
	}
	return nil
}

func (d Delivery) Validate(taskName string) error {
	if d.Type == "" {
		if len(d.To) > 0 {
			return fmt.Errorf("task %q: delivery.type is required when recipients are configured", taskName)
		}
		return nil
	}
	if d.Type != "email" {
		return fmt.Errorf("task %q: unsupported delivery type %q", taskName, d.Type)
	}
	if len(d.To) == 0 {
		return fmt.Errorf("task %q: delivery.to requires at least one email address", taskName)
	}
	for _, recipient := range d.To {
		address, err := mail.ParseAddress(recipient)
		if err != nil || address.Address != recipient {
			return fmt.Errorf("task %q: invalid delivery email address %q", taskName, recipient)
		}
	}
	allowed := map[string]bool{"success": true, "failed": true, "timeout": true}
	for _, status := range d.On {
		if !allowed[status] {
			return fmt.Errorf("task %q: unsupported delivery status %q", taskName, status)
		}
	}
	return nil
}

func (d Delivery) ShouldNotify(status string) bool {
	if d.Type != "email" || len(d.To) == 0 {
		return false
	}
	for _, configured := range d.On {
		if configured == status {
			return true
		}
	}
	return false
}

func (d Delivery) IncludesOutput() bool {
	return d.IncludeOutput == nil || *d.IncludeOutput
}
