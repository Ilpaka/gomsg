package httptransport

import (
	"log/slog"
	"net/http"
	"strings"

	"goflow/backend/internal/config"
	"goflow/backend/internal/service"
	"goflow/backend/internal/transport/http/handler"
	"goflow/backend/internal/transport/http/middleware"
)

// Deps are transport-level dependencies supplied by the app layer (typically from app.Container).
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	Auth   *service.AuthService
}

// Register wires HTTP routes. Handlers stay thin; domain logic lives in services.
func Register(mux *http.ServeMux, deps *Deps) {
	mux.Handle("GET /health", healthHandler(deps))

	if deps.Auth != nil {
		ah := handler.NewAuth(deps.Auth, deps.Logger)
		secret := []byte(strings.TrimSpace(deps.Config.JWT.Secret))

		mux.Handle("POST /auth/register", http.HandlerFunc(ah.Register))
		mux.Handle("POST /auth/login", http.HandlerFunc(ah.Login))
		mux.Handle("POST /auth/refresh", http.HandlerFunc(ah.Refresh))
		mux.Handle("POST /auth/logout", http.HandlerFunc(ah.Logout))
		mux.Handle("POST /auth/logout-all", middleware.RequireBearerAuth(secret, deps.Logger)(http.HandlerFunc(ah.LogoutAll)))
	}
}

func healthHandler(_ *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
