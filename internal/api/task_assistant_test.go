package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type testTaskAssistant struct {
	lastPrompt string
}

func (a *testTaskAssistant) Complete(_ context.Context, prompt string) (string, error) {
	a.lastPrompt = prompt
	if strings.Contains(prompt, "Current draft:") {
		return `{"reply":"你希望简报多长？","ready":true,"missing_fields":[],"quick_replies":["3 分钟速读","详细报告"],"draft":{"name":"每日 AI 简报","description":"每日整理 AI 新闻","schedule":"0 8 * * *","schedule_label":"每天 08:00","timezone":"Asia/Shanghai","prompt":"整理当天最重要的 AI 新闻，列出五条要点并说明来源。","notify_email":false,"email_events":"all","recipient":""}}`, nil
	}
	return "Five important AI updates.", nil
}

func TestTaskAssistantPlanAndTest(t *testing.T) {
	store := storage.NewMemory()
	assistant := &testTaskAssistant{}
	handler := New(Options{
		Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: make(map[string]task.Task)},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultTimezone: "Asia/Shanghai",
		ProviderConfigured: true, Assistant: assistant,
	})

	plan := requestJSON[assistantPlan](t, handler, http.MethodPost, "/api/task-assistant/plan", `{
		"language":"zh","messages":[{"role":"user","content":"每天给我 AI 新闻"}],
		"draft":{"timezone":"Asia/Shanghai","email_events":"all"}
	}`, http.StatusOK)
	if !plan.Ready || plan.Draft.Name != "每日 AI 简报" || plan.Draft.Schedule != "0 8 * * *" || len(plan.QuickReplies) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(assistant.lastPrompt, "每天给我 AI 新闻") {
		t.Fatalf("prompt did not contain conversation: %q", assistant.lastPrompt)
	}

	result := requestJSON[assistantTestJob](t, handler, http.MethodPost, "/api/task-assistant/test", `{
		"name":"每日 AI 简报","description":"每日整理 AI 新闻","schedule":"0 8 * * *",
		"schedule_label":"每天 08:00","timezone":"Asia/Shanghai",
		"prompt":"整理当天最重要的 AI 新闻，列出五条要点并说明来源。",
		"notify_email":false,"email_events":"all","recipient":""
	}`, http.StatusAccepted)
	for deadline := time.Now().Add(time.Second); result.Status == "running" && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
		result = requestJSON[assistantTestJob](t, handler, http.MethodGet, "/api/task-assistant/test/"+result.ID, "", http.StatusOK)
	}
	if result.Status != "success" || result.Output != "Five important AI updates." {
		t.Fatalf("result = %#v", result)
	}
}
