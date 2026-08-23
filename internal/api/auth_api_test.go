package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

type fixedAuth struct{ user auth.User }

func (a fixedAuth) Register(context.Context, string, string, string) (auth.User, error) {
	return a.user, nil
}
func (a fixedAuth) VerifyEmail(context.Context, string) (auth.User, error) { return a.user, nil }
func (a fixedAuth) Login(context.Context, string, string) (auth.User, string, time.Time, error) {
	return a.user, "valid", time.Now().Add(time.Hour), nil
}
func (a fixedAuth) Authenticate(_ context.Context, token string) (auth.User, error) {
	if token != "valid" {
		return auth.User{}, auth.ErrNotFound
	}
	return a.user, nil
}
func (a fixedAuth) Logout(context.Context, string) error { return nil }

func TestAuthenticatedTaskIsolation(t *testing.T) {
	store := storage.NewMemory()
	owned, _ := store.CreateTask(context.Background(), task.Task{OwnerID: "user_a", Name: "Mine", Schedule: "0 8 * * *", Prompt: "mine"})
	_, _ = store.CreateTask(context.Background(), task.Task{OwnerID: "user_b", Name: "Other", Schedule: "0 9 * * *", Prompt: "other"})
	handler := New(Options{Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: map[string]task.Task{}}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: fixedAuth{user: auth.User{ID: "user_a", Email: "a@example.com", EmailVerified: true}}})

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/tasks", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(owned.ID)) || bytes.Contains(response.Body.Bytes(), []byte("Other")) {
		t.Fatalf("tasks response = %d %s", response.Code, response.Body.String())
	}
}

func TestRunHistoryRemainsVisibleAfterTaskDeletion(t *testing.T) {
	store := storage.NewMemory()
	owned, _ := store.CreateTask(context.Background(), task.Task{OwnerID: "user_a", Name: "Mine", Schedule: "0 8 * * *", Prompt: "mine"})
	handler := New(Options{Store: store, Runner: testRunner{store}, Scheduler: &testScheduler{tasks: map[string]task.Task{}}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: fixedAuth{user: auth.User{ID: "user_a", Email: "a@example.com", EmailVerified: true}}})

	authenticated := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	started := authenticated(http.MethodPost, "/api/tasks/"+owned.ID+"/run")
	if started.Code != http.StatusAccepted {
		t.Fatalf("start status = %d: %s", started.Code, started.Body.String())
	}
	var run execution.Execution
	if err := json.NewDecoder(started.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	deleted := authenticated(http.MethodDelete, "/api/tasks/"+owned.ID)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", deleted.Code, deleted.Body.String())
	}

	listed := authenticated(http.MethodGet, "/api/executions")
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(run.ID)) {
		t.Fatalf("history response = %d %s", listed.Code, listed.Body.String())
	}
	found := authenticated(http.MethodGet, "/api/executions/"+run.ID)
	if found.Code != http.StatusOK {
		t.Fatalf("execution response = %d %s", found.Code, found.Body.String())
	}
}
