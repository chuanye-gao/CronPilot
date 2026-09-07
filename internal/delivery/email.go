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

const emailStyles = `.result h1{font-size:22px;margin:18px 0 10px;color:#eef3ef;line-height:1.3}
.result h2{font-size:19px;margin:16px 0 8px;color:#eef3ef;line-height:1.3}
.result h3{font-size:17px;margin:14px 0 6px;color:#eef3ef;line-height:1.3}
.result h4,.result h5,.result h6{font-size:15px;margin:12px 0 6px;color:#eef3ef}
.result p{margin:10px 0}
.result ul,.result ol{margin:10px 0;padding-left:24px}
.result li{margin:4px 0;line-height:1.6}
.result a{color:#7ee787;text-decoration:underline}
.result code{background:#1a211e;border-radius:4px;padding:2px 5px;font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:13px;color:#e2e8e4}
.result pre{background:#0b0f0d;border:1px solid #29302c;border-radius:8px;padding:14px 16px;overflow-x:auto;margin:12px 0}
.result pre code{background:none;padding:0;font-size:13px;color:#c9d1cc}
.result blockquote{border-left:3px solid #3a423e;margin:12px 0;padding:2px 0 2px 14px;color:#9aa49e}
.result hr{border:0;border-top:1px solid #252b28;margin:18px 0}
.result table{border-collapse:collapse;margin:12px 0;width:100%}
.result th,.result td{border:1px solid #29302c;padding:8px 10px;font-size:14px;text-align:left}
.result th{background:#161b19;color:#eef3ef}
.result img{max-width:100%}`

func executionHTML(value task.Task, run execution.Execution, result string) string {
	finished := "—"
	if run.FinishedAt != nil {
		finished = run.FinishedAt.Format(time.RFC1123Z)
	}
	statusColor := "#b9f227"
	if run.Status != execution.StatusSuccess {
		statusColor = "#ff7f79"
	}
	rendered := renderMarkdown(result)
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>%s</style></head><body style="margin:0;background:#0b0e0d;color:#eef3ef;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif"><div style="max-width:640px;margin:0 auto;padding:36px 20px"><div style="font-size:20px;font-weight:700;margin-bottom:30px">CronPilot</div><div style="background:#111513;border:1px solid #252b28;border-radius:14px;overflow:hidden"><div style="padding:26px 28px;border-bottom:1px solid #252b28"><div style="color:%s;font-size:12px;font-weight:700;letter-spacing:.12em">%s</div><h1 style="font-size:24px;margin:10px 0 8px">%s</h1><p style="color:#8b9690;margin:0;line-height:1.6;font-size:14px">%s</p></div><div style="padding:22px 28px"><table style="width:100%%;font-size:13px;color:#8b9690"><tr><td style="padding:5px 0">Started</td><td style="text-align:right;color:#eef3ef">%s</td></tr><tr><td style="padding:5px 0">Finished</td><td style="text-align:right;color:#eef3ef">%s</td></tr><tr><td style="padding:5px 0">Attempts</td><td style="text-align:right;color:#eef3ef">%d</td></tr><tr><td style="padding:5px 0">Execution</td><td style="text-align:right;color:#eef3ef">%s</td></tr></table><div style="margin-top:22px;color:#68736d;font-size:11px;letter-spacing:.12em">RESULT</div><div class="result" style="margin-top:14px;font-size:15px;line-height:1.7;color:#d7deda">%s</div></div></div><p style="color:#59635d;font-size:12px;text-align:center;margin-top:20px">Sent by CronPilot · AI work, right on time.</p></div></body></html>`, emailStyles, statusColor, html.EscapeString(strings.ToUpper(string(run.Status))), html.EscapeString(value.Name), html.EscapeString(value.Description), html.EscapeString(run.StartedAt.Format(time.RFC1123Z)), html.EscapeString(finished), run.Attempts, html.EscapeString(run.ID), rendered)
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
