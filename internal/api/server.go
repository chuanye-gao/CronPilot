package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

//go:embed static
var staticFiles embed.FS

type TaskRunner interface {
	StartTask(context.Context, task.Task) (execution.Execution, error)
}

type TaskScheduler interface {
	Upsert(task.Task) error
	Remove(string)
	NextRun(string) time.Time
}

type EmailService interface {
	Configured() bool
	SendTest(context.Context, string) error
}

type TaskAssistant interface {
	Complete(context.Context, string) (string, error)
}

type AuthService interface {
	Register(context.Context, string, string, string) (auth.User, error)
	VerifyEmail(context.Context, string) (auth.User, error)
	Login(context.Context, string, string) (auth.User, string, time.Time, error)
	Authenticate(context.Context, string) (auth.User, error)
	Logout(context.Context, string) error
}

type Options struct {
	Store               storage.Store
	Runner              TaskRunner
	Scheduler           TaskScheduler
	Logger              *slog.Logger
	DefaultTimezone     string
	Model               string
	FallbackModel       string
	FallbackConfigured  bool
	ProviderConfigured  bool
	Email               EmailService
	Assistant           TaskAssistant
	TaskExecutor        TaskAssistant
	Auth                AuthService
	Readiness           func(context.Context) error
	Storage             string
	WebSearchConfigured bool
	WebSearchProvider   string
	WebSearchHealth     func(context.Context) error
}

type Server struct {
	store               storage.Store
	runner              TaskRunner
	scheduler           TaskScheduler
	logger              *slog.Logger
	defaultTimezone     string
	model               string
	fallbackModel       string
	fallbackConfigured  bool
	providerConfigured  bool
	email               EmailService
	assistant           TaskAssistant
	taskExecutor        TaskAssistant
	auth                AuthService
	readiness           func(context.Context) error
	storage             string
	webSearchConfigured bool
	webSearchProvider   string
	webSearchHealth     func(context.Context) error
	assistantTestsMu    sync.RWMutex
	assistantTests      map[string]assistantTestJob
}

type taskResponse struct {
	task.Task
	NextRun *time.Time `json:"next_run,omitempty"`
}

func New(options Options) http.Handler {
	server := &Server{
		store:               options.Store,
		runner:              options.Runner,
		scheduler:           options.Scheduler,
		logger:              options.Logger,
		defaultTimezone:     options.DefaultTimezone,
		model:               options.Model,
		fallbackModel:       options.FallbackModel,
		fallbackConfigured:  options.FallbackConfigured,
		providerConfigured:  options.ProviderConfigured,
		email:               options.Email,
		assistant:           options.Assistant,
		taskExecutor:        options.TaskExecutor,
		auth:                options.Auth,
		readiness:           options.Readiness,
		storage:             options.Storage,
		webSearchConfigured: options.WebSearchConfigured,
		webSearchProvider:   options.WebSearchProvider,
		webSearchHealth:     options.WebSearchHealth,
		assistantTests:      make(map[string]assistantTestJob),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", server.live)
	mux.HandleFunc("GET /health/ready", server.ready)
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("POST /api/auth/register", server.register)
	mux.HandleFunc("POST /api/auth/login", server.login)
	mux.HandleFunc("POST /api/auth/verify", server.verifyEmail)
	mux.Handle("GET /api/auth/me", server.requireAuth(http.HandlerFunc(server.me)))
	mux.Handle("POST /api/auth/logout", server.requireAuth(http.HandlerFunc(server.logout)))
	mux.Handle("GET /api/tasks", server.requireAuth(http.HandlerFunc(server.listTasks)))
	mux.Handle("POST /api/tasks", server.requireAuth(http.HandlerFunc(server.createTask)))
	mux.Handle("GET /api/tasks/{id}", server.requireAuth(http.HandlerFunc(server.getTask)))
	mux.Handle("PUT /api/tasks/{id}", server.requireAuth(http.HandlerFunc(server.updateTask)))
	mux.Handle("DELETE /api/tasks/{id}", server.requireAuth(http.HandlerFunc(server.deleteTask)))
	mux.Handle("POST /api/tasks/{id}/run", server.requireAuth(http.HandlerFunc(server.runTask)))
	mux.Handle("GET /api/tasks/{id}/executions", server.requireAuth(http.HandlerFunc(server.listTaskExecutions)))
	mux.Handle("GET /api/executions", server.requireAuth(http.HandlerFunc(server.listExecutions)))
	mux.Handle("GET /api/executions/{id}", server.requireAuth(http.HandlerFunc(server.getExecution)))
	mux.Handle("GET /api/email/status", server.requireAuth(http.HandlerFunc(server.emailStatus)))
	mux.Handle("POST /api/email/test", server.requireAuth(http.HandlerFunc(server.testEmail)))
	mux.Handle("POST /api/task-assistant/plan", server.requireAuth(http.HandlerFunc(server.planTask)))
	mux.Handle("POST /api/task-assistant/test", server.requireAuth(http.HandlerFunc(server.testTaskDraft)))
	mux.Handle("GET /api/task-assistant/test/{id}", server.requireAuth(http.HandlerFunc(server.getTaskDraftTest)))
	registerMethodFallbacks(mux)

	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("prepare embedded web console: " + err.Error())
	}
	mux.Handle("/", http.FileServerFS(assets))
	return securityHeaders(requestLogging(options.Logger, recoverPanics(options.Logger, mux)))
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account service is not configured")
		return
	}
	var request struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	user, err := s.auth.Register(ctx, request.Name, request.Email, request.Password)
	if err != nil {
		s.authError(w, "register account", err)
		return
	}
	if ownerStore, ok := s.store.(interface {
		ClaimUnownedTasks(context.Context, string) error
	}); ok {
		if err := ownerStore.ClaimUnownedTasks(ctx, user.ID); err != nil {
			s.internalError(w, "claim existing tasks", err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "verification_required": true})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account service is not configured")
		return
	}
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, token, expiresAt, err := s.auth.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		s.authError(w, "login", err)
		return
	}
	s.setSessionCookie(w, r, token, expiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account service is not configured")
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.auth.VerifyEmail(r.Context(), request.Token)
	if err != nil {
		s.authError(w, "verify email", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "verified": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": requestUser(r.Context())})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && s.auth != nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.internalError(w, "logout", err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.readyError(r.Context()); err != nil {
		s.logger.Error("readiness check failed", "request_id", requestID(r.Context()), "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	statusCode := http.StatusOK
	if err := s.readyError(r.Context()); err != nil {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}
	webSearchStatus := "disabled"
	if s.webSearchConfigured {
		webSearchStatus = "healthy"
		if s.webSearchHealth != nil {
			checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			if err := s.webSearchHealth(checkCtx); err != nil {
				webSearchStatus = "unavailable"
				s.logger.Warn("web search health check failed", "request_id", requestID(r.Context()), "error", err)
			}
			cancel()
		}
	}
	writeJSON(w, statusCode, map[string]any{
		"status":                status,
		"model":                 s.model,
		"fallback_model":        s.fallbackModel,
		"fallback_configured":   s.fallbackConfigured,
		"provider_configured":   s.providerConfigured,
		"timezone":              s.defaultTimezone,
		"email_configured":      s.emailConfigured(),
		"storage":               s.storage,
		"web_search_configured": s.webSearchConfigured,
		"web_search_status":     webSearchStatus,
		"web_search_provider":   s.webSearchProvider,
	})
}

func (s *Server) readyError(ctx context.Context) error {
	if s.readiness == nil {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.readiness(checkCtx)
}

func (s *Server) emailStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": s.emailConfigured(),
		"provider":   "SMTP",
	})
}

func (s *Server) testEmail(w http.ResponseWriter, r *http.Request) {
	if !s.emailConfigured() {
		writeError(w, http.StatusServiceUnavailable, "email delivery is not configured; add SMTP settings and restart CronPilot")
		return
	}
	var request struct {
		To string `json:"to"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.email.SendTest(ctx, request.To); err != nil {
		s.logger.Error("send test email", "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent", "to": request.To})
}

func (s *Server) emailConfigured() bool {
	return s.email != nil && s.email.Configured()
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListTasks(r.Context())
	if err != nil {
		s.internalError(w, "list tasks", err)
		return
	}
	ownerID := requestUser(r.Context()).ID
	result := make([]taskResponse, 0, len(tasks))
	for _, value := range tasks {
		if ownerID != "" && value.OwnerID != ownerID {
			continue
		}
		result = append(result, s.taskResponse(value))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var value task.Task
	if err := decodeJSON(r, &value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value.ID = ""
	value.OwnerID = requestUser(r.Context()).ID
	value.ApplyDefaults(s.defaultTimezone)
	if err := value.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.CreateTask(r.Context(), value)
	if err != nil {
		s.internalError(w, "create task", err)
		return
	}
	if err := s.scheduler.Upsert(created); err != nil {
		_ = s.store.DeleteTask(r.Context(), created.ID)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.taskResponse(created))
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	value, ok := s.findTask(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.taskResponse(value))
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	current, ok := s.findTask(w, r)
	if !ok {
		return
	}
	var value task.Task
	if err := decodeJSON(r, &value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value.ID = current.ID
	value.OwnerID = current.OwnerID
	value.CreatedAt = current.CreatedAt
	value.ApplyDefaults(s.defaultTimezone)
	if err := value.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.scheduler.Upsert(value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.UpdateTask(r.Context(), value)
	if err != nil {
		_ = s.scheduler.Upsert(current)
		s.internalError(w, "update task", err)
		return
	}
	writeJSON(w, http.StatusOK, s.taskResponse(updated))
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	value, ok := s.findTask(w, r)
	if !ok {
		return
	}
	s.scheduler.Remove(value.ID)
	if err := s.store.DeleteTask(r.Context(), value.ID); err != nil {
		_ = s.scheduler.Upsert(value)
		s.internalError(w, "delete task", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	value, ok := s.findTask(w, r)
	if !ok {
		return
	}
	run, err := s.runner.StartTask(context.Background(), value)
	if err != nil {
		s.internalError(w, "start task", err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listTaskExecutions(w http.ResponseWriter, r *http.Request) {
	value, ok := s.findTask(w, r)
	if !ok {
		return
	}
	s.listExecutionsFor(w, r, value.ID)
}

func (s *Server) listExecutions(w http.ResponseWriter, r *http.Request) {
	s.listExecutionsFor(w, r, "")
}

func (s *Server) listExecutionsFor(w http.ResponseWriter, r *http.Request, taskID string) {
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	ownerID := requestUser(r.Context()).ID
	if ownerID != "" && taskID == "" {
		runs, err := s.store.ListExecutionsByOwner(r.Context(), ownerID, limit)
		if err != nil {
			s.internalError(w, "list executions by owner", err)
			return
		}
		writeJSON(w, http.StatusOK, runs)
		return
	}
	runs, err := s.store.ListExecutions(r.Context(), taskID, limit)
	if err != nil {
		s.internalError(w, "list executions", err)
		return
	}
	if ownerID == "" || taskID != "" {
		writeJSON(w, http.StatusOK, runs)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getExecution(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetExecution(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		s.internalError(w, "get execution", err)
		return
	}
	if userID := requestUser(r.Context()).ID; userID != "" {
		if run.OwnerID != userID && (run.OwnerID != "" || !s.executionBelongsTo(r.Context(), run, userID)) {
			writeError(w, http.StatusNotFound, "execution not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) executionBelongsTo(ctx context.Context, run execution.Execution, ownerID string) bool {
	value, err := s.store.GetTask(ctx, run.TaskID)
	return err == nil && value.OwnerID == ownerID
}

func (s *Server) findTask(w http.ResponseWriter, r *http.Request) (task.Task, bool) {
	value, err := s.store.GetTask(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return task.Task{}, false
	}
	if err != nil {
		s.internalError(w, "get task", err)
		return task.Task{}, false
	}
	if userID := requestUser(r.Context()).ID; userID != "" && value.OwnerID != userID {
		writeError(w, http.StatusNotFound, "task not found")
		return task.Task{}, false
	}
	return value, true
}

func (s *Server) taskResponse(value task.Task) taskResponse {
	response := taskResponse{Task: value}
	if next := s.scheduler.NextRun(value.ID); !next.IsZero() {
		response.NextRun = &next
	}
	return response
}

func (s *Server) internalError(w http.ResponseWriter, operation string, err error) {
	errorID := id.New("err")
	s.logger.Error(operation, "request_id", w.Header().Get("X-Request-ID"), "error_id", errorID, "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error", "error_id": errorID})
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("invalid request: request body must contain exactly one JSON value")
		}
		return fmt.Errorf("invalid request: request body must contain exactly one JSON value: %w", err)
	}
	return nil
}

func registerMethodFallbacks(mux *http.ServeMux) {
	for path, methods := range map[string]string{
		"/health/live":                  "GET, HEAD",
		"/health/ready":                 "GET, HEAD",
		"/api/health":                   "GET, HEAD",
		"/api/auth/register":            "POST",
		"/api/auth/login":               "POST",
		"/api/auth/verify":              "POST",
		"/api/auth/me":                  "GET, HEAD",
		"/api/auth/logout":              "POST",
		"/api/tasks":                    "GET, HEAD, POST",
		"/api/tasks/{id}":               "GET, HEAD, PUT, DELETE",
		"/api/tasks/{id}/run":           "POST",
		"/api/tasks/{id}/executions":    "GET, HEAD",
		"/api/executions":               "GET, HEAD",
		"/api/executions/{id}":          "GET, HEAD",
		"/api/email/status":             "GET, HEAD",
		"/api/email/test":               "POST",
		"/api/task-assistant/plan":      "POST",
		"/api/task-assistant/test":      "POST",
		"/api/task-assistant/test/{id}": "GET, HEAD",
	} {
		allowed := methods
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", allowed)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		})
	}
}

func parseLimit(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 {
		return fallback
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

const sessionCookieName = "cronpilot_session"

type userContextKey struct{}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, auth.User{})))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !requestOriginAllowed(r) {
			writeError(w, http.StatusForbidden, "request origin is not allowed")
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		user, err := s.auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
				HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode})
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func requestUser(ctx context.Context) auth.User {
	value, _ := ctx.Value(userContextKey{}).(auth.User)
	return value
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()),
		HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode,
	})
}

func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func requestOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host) && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (s *Server) authError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "this email address is already registered")
	case errors.Is(err, auth.ErrInvalidCredential):
		writeError(w, http.StatusUnauthorized, "invalid email address or password")
	case errors.Is(err, auth.ErrEmailNotVerified):
		writeError(w, http.StatusForbidden, "verify your email address before signing in")
	case errors.Is(err, auth.ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "verification link is invalid or expired")
	case strings.Contains(err.Error(), "send verification email"):
		s.logger.Error(operation, "error", err)
		writeError(w, http.StatusBadGateway, "could not send the verification email")
	case strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must contain") || strings.Contains(err.Error(), "must not exceed"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.internalError(w, operation, err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
