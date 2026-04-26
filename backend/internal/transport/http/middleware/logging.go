package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logging logs one line per request: method, path, status, duration, optional request_id.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if log == nil {
				next.ServeHTTP(w, r)
				return
			}
			rid := r.Header.Get("X-Request-ID")
			if rid == "" {
				var b [8]byte
				_, _ = rand.Read(b[:])
				rid = hex.EncodeToString(b[:])
			}
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			w.Header().Set("X-Request-ID", rid)
			start := time.Now()
			next.ServeHTTP(rec, r)
			log.Info("http request",
				"message", "http request completed",
				"http.method", r.Method,
				"http.path", r.URL.Path,
				"http.status", rec.status,
				"http.duration_ms", time.Since(start).Milliseconds(),
				"request_id", rid,
			)
		})
	}
}
