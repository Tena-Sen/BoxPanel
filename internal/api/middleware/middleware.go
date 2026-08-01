// Package middleware provides chi-style HTTP middlewares.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Recoverer catches panics and returns 500.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "err", rec, "stack", string(debug.Stack()))
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logger logs each request.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path,
			"status", ww.status, "dur", time.Since(start).String())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 透传到底层 ResponseWriter，保证 SSE/分块响应在中间件包装后仍可 flush。
// 不实现的话 w.(http.Flusher) 断言会失败，导致 /api/logs、/api/traffic 等流式接口报 "streaming unsupported"。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap 暴露底层 ResponseWriter，让 http.Hijacker 等接口能被类型断言找到（Go 1.20+）。
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
