package api

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/id"
)

type requestIDKey struct{}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		currentRequestID := id.New("req")
		ctx := context.WithValue(r.Context(), requestIDKey{}, currentRequestID)
		w.Header().Set("X-Request-ID", currentRequestID)
		response := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r.WithContext(ctx))
		attributes := []any{
			"request_id", currentRequestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", response.status,
			"bytes", response.bytes,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
			logger.Debug("http health check", attributes...)
			return
		}
		logger.Info("http request", attributes...)
	})
}

func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				errorID := id.New("err")
				logger.Error("http panic",
					"request_id", requestID(r.Context()),
					"error_id", errorID,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error", "error_id": errorID})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
