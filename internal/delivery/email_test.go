package delivery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type captureSender struct {
	message Message
	err     error
}

func (s *captureSender) Send(_ context.Context, message Message) error {
	s.message = message
	return s.err
}

func TestEmailSendTest(t *testing.T) {
	sender := &captureSender{}
	email := NewEmail(sender, "CronPilot <sender@example.com>")
	if err := email.SendTest(context.Background(), "owner@example.com"); err != nil {
		t.Fatalf("SendTest() error = %v", err)
	}
	if sender.message.To[0] != "owner@example.com" || !strings.Contains(sender.message.HTML, "Email delivery is ready") {
		t.Fatalf("message = %#v", sender.message)
	}
	if err := email.SendTest(context.Background(), "not-an-email"); err == nil {
		t.Fatal("SendTest() accepted an invalid address")
	}
}

func TestEmailDeliverExecution(t *testing.T) {
	sender := &captureSender{}
	email := NewEmail(sender, "sender@example.com")
	finished := time.Now().UTC()
	includeOutput := true
	value := task.Task{
		Name: "Daily brief", Description: "Morning news",
		Delivery: task.Delivery{Type: "email", To: []string{"owner@example.com"}, IncludeOutput: &includeOutput},
	}
	run := execution.Execution{ID: "run_test", TaskName: value.Name, Status: execution.StatusSuccess, StartedAt: finished.Add(-time.Second), FinishedAt: &finished, Output: "important result", Attempts: 1}
	if err := email.Deliver(context.Background(), value, run); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if !strings.Contains(sender.message.Subject, "Daily brief") || !strings.Contains(sender.message.Text, "important result") {
		t.Fatalf("message = %#v", sender.message)
	}
}
