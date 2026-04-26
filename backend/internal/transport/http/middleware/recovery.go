package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/pkg/response"
)

// Recovery catches panics in downstream handlers, logs the stack, and returns a JSON 500.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if log != nil {
						log.Error("panic recovered",
							"recover", rec,
							"path", r.URL.Path,
							"method", r.Method,
							"stack", string(debug.Stack()),
						)
					}
					response.WriteError(w, r, log, apperr.Internal("internal server error", nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
