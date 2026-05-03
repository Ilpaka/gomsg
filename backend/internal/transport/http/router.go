package httptransport

import (
	"log/slog"
	"net/http"
	"strings"

	"goflow/backend/internal/config"
	"goflow/backend/internal/observability/metrics"
	"goflow/backend/internal/service"
	"goflow/backend/internal/transport/http/docs"
	"goflow/backend/internal/transport/http/handler"
	"goflow/backend/internal/transport/http/middleware"
)

func chainLimit(l *middleware.IPRateLimiter, h http.Handler) http.Handler {
	if l == nil || h == nil {
		return h
	}
	return l.Handler(h)
}

// WithRateLimit exposes rate-limit wrapping for other packages (e.g. app wiring).
func WithRateLimit(l *middleware.IPRateLimiter, h http.Handler) http.Handler {
	return chainLimit(l, h)
}

// Deps are transport-level dependencies supplied by the app layer (typically from app.Container).
type Deps struct {
	Config   *config.Config
	Logger   *slog.Logger
	Metrics  *metrics.M
	Auth     *service.AuthService
	Users    *service.UserService
	Chats    *service.ChatService
	Messages *service.MessageService
	// WS is GET /ws/connect (websocket); nil disables the route.
	WS http.Handler
	// WSTicket is POST /ws/ticket (Bearer); nil disables the route.
	WSTicket http.Handler

	LimitRegister    *middleware.IPRateLimiter
	LimitLogin       *middleware.IPRateLimiter
	LimitMessageSend *middleware.IPRateLimiter
	LimitWSConnect   *middleware.IPRateLimiter
}

// Register wires HTTP routes. Handlers stay thin; domain logic lives in services.
func Register(mux *http.ServeMux, deps *Deps) {
	docs.Register(mux)

	mux.Handle("GET /health", healthHandler(deps))

	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics.Handler())
	}

	secret := []byte(strings.TrimSpace(deps.Config.JWT.Secret))

	if deps.Auth != nil {
		ah := handler.NewAuth(deps.Auth, deps.Logger, deps.Metrics)

		mux.Handle("POST /auth/register", chainLimit(deps.LimitRegister, http.HandlerFunc(ah.Register)))
		mux.Handle("POST /auth/login", chainLimit(deps.LimitLogin, http.HandlerFunc(ah.Login)))
		mux.Handle("POST /auth/refresh", http.HandlerFunc(ah.Refresh))
		mux.Handle("POST /auth/logout", http.HandlerFunc(ah.Logout))
		mux.Handle("POST /auth/logout-all", middleware.RequireBearerAuth(secret, deps.Logger)(http.HandlerFunc(ah.LogoutAll)))
	}

	if deps.Users != nil {
		uh := handler.NewUsers(deps.Users, deps.Logger)
		authUser := middleware.RequireBearerAuth(secret, deps.Logger)

		mux.Handle("GET /users/me", authUser(http.HandlerFunc(uh.Me)))
		mux.Handle("PATCH /users/me", authUser(http.HandlerFunc(uh.PatchMe)))
		mux.Handle("GET /users/search", authUser(http.HandlerFunc(uh.Search)))
		mux.Handle("GET /users/{id}", authUser(http.HandlerFunc(uh.ByID)))
	}

	if deps.Chats != nil {
		ch := handler.NewChats(deps.Chats, deps.Logger)
		authChat := middleware.RequireBearerAuth(secret, deps.Logger)

		mux.Handle("GET /chats", authChat(http.HandlerFunc(ch.List)))
		mux.Handle("POST /chats/direct", authChat(http.HandlerFunc(ch.CreateDirect)))
		mux.Handle("POST /chats/group", authChat(http.HandlerFunc(ch.CreateGroup)))
		mux.Handle("GET /chats/{chat_id}/members", authChat(http.HandlerFunc(ch.Members)))
		mux.Handle("POST /chats/{chat_id}/members", authChat(http.HandlerFunc(ch.AddMembers)))
		mux.Handle("DELETE /chats/{chat_id}/members/{user_id}", authChat(http.HandlerFunc(ch.RemoveMember)))
		mux.Handle("GET /chats/{chat_id}", authChat(http.HandlerFunc(ch.Get)))
	}

	if deps.Messages != nil {
		mh := handler.NewMessages(deps.Messages, deps.Logger)
		authMsg := middleware.RequireBearerAuth(secret, deps.Logger)

		mux.Handle("GET /chats/{chat_id}/messages", authMsg(http.HandlerFunc(mh.ListByChat)))
		mux.Handle("POST /chats/{chat_id}/messages", chainLimit(deps.LimitMessageSend, authMsg(http.HandlerFunc(mh.CreateInChat))))
		mux.Handle("GET /messages/{message_id}", authMsg(http.HandlerFunc(mh.Get)))
		mux.Handle("PATCH /messages/{message_id}", authMsg(http.HandlerFunc(mh.Patch)))
		mux.Handle("DELETE /messages/{message_id}", authMsg(http.HandlerFunc(mh.Delete)))
		mux.Handle("POST /messages/{message_id}/read", authMsg(http.HandlerFunc(mh.MarkRead)))
	}

	if deps.WSTicket != nil {
		mux.Handle("POST /ws/ticket", deps.WSTicket)
	}

	if deps.WS != nil {
		mux.Handle("GET /ws/connect", chainLimit(deps.LimitWSConnect, deps.WS))
	}
}

func healthHandler(_ *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// Chain wraps the ServeMux with standard middleware: Prometheus HTTP metrics (outermost),
// panic recovery, and request logging.
// Pass the returned handler to http.Server instead of the raw mux.
func Chain(mux *http.ServeMux, log *slog.Logger, met *metrics.M) http.Handler {
	if mux == nil {
		return http.NotFoundHandler()
	}
	h := http.Handler(mux)
	h = middleware.Recovery(log)(h)
	h = middleware.Logging(log)(h)
	if met != nil {
		h = met.HTTPMiddleware(h)
	}
	return h
}
