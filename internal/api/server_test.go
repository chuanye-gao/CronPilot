package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type testScheduler struct {
	tasks map[string]task.Task
}

func (s *testScheduler) Upsert(value task.Task) error { s.tasks[value.ID] = value; return nil }
func (s *testScheduler) Remove(taskID string)         { delete(s.tasks, taskID) }
func (s *testScheduler) NextRun(string) time.Time     { return time.Now().Add(time.Hour) }

type testRunner struct{ store storage.Store }

func (r testRunner) StartTask(ctx context.Context, value task.Task) (execution.Execution, error) {
	run := execution.New(value)
	if err := r.store.CreateExecution(ctx, run); err != nil {
		return execution.Execution{}, err
	}
	return run, nil
}

type testEmailService struct {
	configured bool
	recipient  string
	err        error
}

func (e *testEmailService) Configured() bool { return e.configured }
func (e *testEmailService) SendTest(_ context.Context, recipient string) error {
	e.recipient = recipient
	return e.err
}

func TestTaskAPIAndManualRun(t *testing.T) {
	store := storage.NewMemory()
	scheduler := &testScheduler{tasks: make(map[string]task.Task)}
	handler := New(Options{
		Store: store, Runner: testRunner{store: store}, Scheduler: scheduler,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultTimezone: "Asia/Shanghai", Model: "test",
	})

	created := requestJSON[taskResponse](t, handler, http.MethodPost, "/api/tasks", `{
		"name":"Daily brief","schedule":"0 8 * * *","prompt":"write it","timeout":"1m",
		"retry":{"max_attempts":2,"delay":"1s"},"enabled":true
	}`, http.StatusCreated)
	if created.ID == "" || created.Timezone != "Asia/Shanghai" || !created.IsEnabled() {
		t.Fatalf("created = %#v", created)
	}
	if _, ok := scheduler.tasks[created.ID]; !ok {
		t.Fatal("task was not scheduled")
	}

	run := requestJSON[execution.Execution](t, handler, http.MethodPost, "/api/tasks/"+created.ID+"/run", "", http.StatusAccepted)
	if run.TaskID != created.ID || run.Status != execution.StatusPending {
		t.Fatalf("run = %#v", run)
	}
	runs := requestJSON[[]execution.Execution](t, handler, http.MethodGet, "/api/executions", "", http.StatusOK)
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v", runs)
	}

	request(t, handler, http.MethodDelete, "/api/tasks/"+created.ID, "", http.StatusNoContent)
	request(t, handler, http.MethodGet, "/api/tasks/"+created.ID, "", http.StatusNotFound)
}

func TestServesWebConsoleAndHealth(t *testing.T) {
	store := storage.NewMemory()
	handler := New(Options{Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: make(map[string]task.Task)}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	response := request(t, handler, http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(response.Body.String(), "CronPilot") {
		t.Fatal("web console did not contain product content")
	}
	health := requestJSON[map[string]any](t, handler, http.MethodGet, "/api/health", "", http.StatusOK)
	if health["status"] != "ok" {
		t.Fatalf("health = %#v", health)
	}
	live := request(t, handler, http.MethodGet, "/health/live", "", http.StatusOK)
	if live.Header().Get("X-Request-ID") == "" {
		t.Fatal("health response did not include a request ID")
	}
}

func TestReadinessReportsDependencyFailure(t *testing.T) {
	store := storage.NewMemory()
	handler := New(Options{
		Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: make(map[string]task.Task)},
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Readiness: func(context.Context) error { return errors.New("database unavailable") },
	})
	response := requestJSON[map[string]string](t, handler, http.MethodGet, "/health/ready", "", http.StatusServiceUnavailable)
	if response["status"] != "not_ready" {
		t.Fatalf("readiness = %#v", response)
	}
}

func TestEmailTestAPI(t *testing.T) {
	store := storage.NewMemory()
	email := &testEmailService{configured: true}
	handler := New(Options{
		Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: make(map[string]task.Task)},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Email: email,
	})
	response := requestJSON[map[string]string](t, handler, http.MethodPost, "/api/email/test", `{"to":"owner@example.com"}`, http.StatusOK)
	if response["status"] != "sent" || email.recipient != "owner@example.com" {
		t.Fatalf("response = %#v, recipient = %q", response, email.recipient)
	}
}

func TestRejectsTrailingJSONValue(t *testing.T) {
	store := storage.NewMemory()
	handler := New(Options{
		Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: make(map[string]task.Task)},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request(t, handler, http.MethodPost, "/api/tasks", `{
		"name":"First","schedule":"0 8 * * *","prompt":"write it"
	}{"name":"Second"}`, http.StatusBadRequest)
	tasks, err := store.ListTasks(context.Background())
	if err != nil || len(tasks) != 0 {
		t.Fatalf("trailing JSON created tasks: %#v, %v", tasks, err)
	}
}

func TestKnownRouteRejectsUnsupportedMethod(t *testing.T) {
	handler := New(Options{
		Store: storage.NewMemory(), Runner: testRunner{store: storage.NewMemory()},
		Scheduler: &testScheduler{tasks: make(map[string]task.Task)},
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	response := request(t, handler, http.MethodPut, "/api/auth/login", "", http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != "POST" {
		t.Fatalf("Allow = %q", response.Header().Get("Allow"))
	}
}

func requestJSON[T any](t *testing.T, handler http.Handler, method, path, body string, status int) T {
	t.Helper()
	response := request(t, handler, method, path, body, status)
	var value T
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return value
}

func request(t *testing.T, handler http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("%s %s status = %d, body = %s", method, path, response.Code, response.Body.String())
	}
	return response
}
