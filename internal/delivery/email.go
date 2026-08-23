package delivery

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

const maxEmailOutput = 64 * 1024

type Message struct {
	From    string
	To      []string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(context.Context, Message) error
}

type Email struct {
	sender Sender
	from   string
}

func NewEmail(sender Sender, from string) *Email {
	return &Email{sender: sender, from: strings.TrimSpace(from)}
}

func (e *Email) Configured() bool {
	return e != nil && e.sender != nil && e.from != ""
}

func (e *Email) SendTest(ctx context.Context, recipient string) error {
	if !e.Configured() {
		return fmt.Errorf("email delivery is not configured")
	}
	address, err := mail.ParseAddress(strings.TrimSpace(recipient))
	if err != nil || address.Address != strings.TrimSpace(recipient) {
		return fmt.Errorf("invalid recipient email address")
	}
	now := time.Now()
	message := Message{
		From:    e.from,
		To:      []string{address.Address},
		Subject: "[CronPilot] Email delivery is ready",
		Text:    fmt.Sprintf("CronPilot email delivery is configured correctly.\n\nTest sent at: %s\nRecipient: %s\n\nYou can now enable email notifications for scheduled AI tasks.", now.Format(time.RFC1123Z), address.Address),
		HTML:    testEmailHTML(address.Address, now),
	}
	if err := e.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("send test email: %w", err)
	}
	return nil
}

func (e *Email) SendVerification(ctx context.Context, recipient, name, verificationURL string) error {
	if !e.Configured() {
		return fmt.Errorf("email delivery is not configured")
	}
	address, err := mail.ParseAddress(strings.TrimSpace(recipient))
	if err != nil || address.Address != strings.TrimSpace(recipient) {
		return fmt.Errorf("invalid recipient email address")
	}
	name = strings.TrimSpace(name)
	message := Message{
		From:    e.from,
		To:      []string{address.Address},
		Subject: "[CronPilot] Verify your email address",
		Text: fmt.Sprintf("Hello %s,\n\nVerify your CronPilot email address by opening this link:\n%s\n\nThis link expires in 30 minutes. If you did not create this account, ignore this email.",
			name, verificationURL),
		HTML: verificationEmailHTML(name, verificationURL),
	}
	if err := e.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

func (e *Email) Deliver(ctx context.Context, value task.Task, run execution.Execution) error {
	if !e.Configured() {
		return fmt.Errorf("email delivery is not configured")
	}
	if len(value.Delivery.To) == 0 {
		return fmt.Errorf("task %q has no email recipients", value.Name)
	}
	subject := fmt.Sprintf("[CronPilot] %s — %s", value.Name, statusLabel(run.Status))
	result := run.Output
	if run.Error != "" {
		result = run.Error
	}
	if !value.Delivery.IncludesOutput() {
		result = "Output is hidden by this task's email notification settings."
	}
	result = truncate(result, maxEmailOutput)
	message := Message{
		From:    e.from,
		To:      value.Delivery.To,
		Subject: subject,
		Text:    executionText(value, run, result),
		HTML:    executionHTML(value, run, result),
	}
	if err := e.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("deliver execution %q by email: %w", run.ID, err)
	}
	return nil
}

func statusLabel(status execution.Status) string {
	switch status {
	case execution.StatusSuccess:
		return "completed successfully"
	case execution.StatusTimeout:
		return "timed out"
	default:
		return "failed"
	}
}

func executionText(value task.Task, run execution.Execution, result string) string {
	finished := "—"
	if run.FinishedAt != nil {
		finished = run.FinishedAt.Format(time.RFC1123Z)
	}
	return fmt.Sprintf("CronPilot task: %s\nStatus: %s\nStarted: %s\nFinished: %s\nAttempts: %d\nExecution: %s\n\nResult\n------\n%s\n", value.Name, strings.ToUpper(string(run.Status)), run.StartedAt.Format(time.RFC1123Z), finished, run.Attempts, run.ID, result)
}

func executionHTML(value task.Task, run execution.Execution, result string) string {
	finished := "—"
	if run.FinishedAt != nil {
		finished = run.FinishedAt.Format(time.RFC1123Z)
	}
	statusColor := "#b9f227"
	if run.Status != execution.StatusSuccess {
		statusColor = "#ff7f79"
	}
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;background:#0b0e0d;color:#eef3ef;font-family:Arial,sans-serif"><div style="max-width:640px;margin:0 auto;padding:36px 20px"><div style="font-size:20px;font-weight:700;margin-bottom:30px">CronPilot</div><div style="background:#111513;border:1px solid #252b28;border-radius:14px;overflow:hidden"><div style="padding:26px 28px;border-bottom:1px solid #252b28"><div style="color:%s;font-size:11px;font-weight:700;letter-spacing:.12em">%s</div><h1 style="font-size:26px;margin:10px 0 8px">%s</h1><p style="color:#8b9690;margin:0;line-height:1.6">%s</p></div><div style="padding:22px 28px"><table style="width:100%%;font-size:12px;color:#8b9690"><tr><td style="padding:5px 0">Started</td><td style="text-align:right;color:#eef3ef">%s</td></tr><tr><td style="padding:5px 0">Finished</td><td style="text-align:right;color:#eef3ef">%s</td></tr><tr><td style="padding:5px 0">Attempts</td><td style="text-align:right;color:#eef3ef">%d</td></tr><tr><td style="padding:5px 0">Execution</td><td style="text-align:right;color:#eef3ef">%s</td></tr></table><div style="margin-top:22px;color:#68736d;font-size:10px;letter-spacing:.12em">RESULT</div><pre style="white-space:pre-wrap;word-break:break-word;background:#0b0f0d;border:1px solid #29302c;border-radius:9px;padding:16px;color:#c9d1cc;font:12px/1.65 monospace">%s</pre></div></div><p style="color:#59635d;font-size:11px;text-align:center;margin-top:20px">Sent by CronPilot · AI work, right on time.</p></div></body></html>`, statusColor, html.EscapeString(strings.ToUpper(string(run.Status))), html.EscapeString(value.Name), html.EscapeString(value.Description), html.EscapeString(run.StartedAt.Format(time.RFC1123Z)), html.EscapeString(finished), run.Attempts, html.EscapeString(run.ID), html.EscapeString(result))
}

func testEmailHTML(recipient string, sentAt time.Time) string {
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;background:#0b0e0d;color:#eef3ef;font-family:Arial,sans-serif"><div style="max-width:600px;margin:0 auto;padding:42px 20px"><div style="font-size:20px;font-weight:700;margin-bottom:30px">CronPilot</div><div style="background:#111513;border:1px solid #2d3726;border-radius:14px;padding:30px"><div style="width:42px;height:42px;line-height:42px;text-align:center;border-radius:50%%;background:#26320e;color:#b9f227;font-size:20px">✓</div><h1 style="font-size:27px;margin:22px 0 10px">Email delivery is ready.</h1><p style="color:#8b9690;line-height:1.7">This test confirms that CronPilot can deliver task notifications to <strong style="color:#eef3ef">%s</strong>.</p><p style="color:#59635d;font-size:11px;margin-top:26px">Sent at %s</p></div></div></body></html>`, html.EscapeString(recipient), html.EscapeString(sentAt.Format(time.RFC1123Z)))
}

func verificationEmailHTML(name, verificationURL string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;background:#0b0e0d;color:#eef3ef;font-family:Arial,sans-serif"><div style="max-width:600px;margin:0 auto;padding:42px 20px"><div style="font-size:20px;font-weight:700;margin-bottom:30px">CronPilot</div><div style="background:#111513;border:1px solid #2d3726;border-radius:14px;padding:30px"><div style="color:#b9f227;font-size:11px;font-weight:700;letter-spacing:.12em">VERIFY EMAIL</div><h1 style="font-size:27px;margin:16px 0 10px">Welcome, %s.</h1><p style="color:#8b9690;line-height:1.7">Confirm this email address to activate your CronPilot workspace.</p><a href="%s" style="display:inline-block;margin-top:18px;padding:13px 20px;color:#111610;background:#c8f135;border-radius:8px;text-decoration:none;font-weight:700">Verify email address</a><p style="color:#59635d;font-size:11px;margin-top:26px">This link expires in 30 minutes. If you did not create this account, ignore this email.</p></div></div></body></html>`,
		html.EscapeString(name), html.EscapeString(verificationURL))
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n\n[Output truncated by CronPilot]"
}
