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
	Config   *config.Config
	Logger   *slog.Logger
	Auth     *service.AuthService
	Users    *service.UserService
	Chats    *service.ChatService
	Messages *service.MessageService
	// WS is GET /ws/connect (websocket); nil disables the route.
	WS http.Handler
}

// Register wires HTTP routes. Handlers stay thin; domain logic lives in services.
func Register(mux *http.ServeMux, deps *Deps) {
	mux.Handle("GET /health", healthHandler(deps))

	secret := []byte(strings.TrimSpace(deps.Config.JWT.Secret))

	if deps.Auth != nil {
		ah := handler.NewAuth(deps.Auth, deps.Logger)

		mux.Handle("POST /auth/register", http.HandlerFunc(ah.Register))
		mux.Handle("POST /auth/login", http.HandlerFunc(ah.Login))
		mux.Handle("POST /auth/refresh", http.HandlerFunc(ah.Refresh))
		mux.Handle("POST /auth/logout", http.HandlerFunc(ah.Logout))
		mux.Handle("POST /auth/logout-all", middleware.RequireBearerAuth(secret, deps.Logger)(http.HandlerFunc(ah.LogoutAll)))
	}

	if deps.Users != nil {
		uh := handler.NewUsers(deps.Users, deps.Logger)
		authUser := middleware.RequireBearerAuth(secret, deps.Logger)

		mux.Handle("GET /users/me", authUser(http.HandlerFunc(uh.Me)))
		mux.Handle("PATCH /users/me", authUser(http.HandlerFunc(uh.PatchMe)))
		mux.Handle("GET /users/search", http.HandlerFunc(uh.Search))
		mux.Handle("GET /users/{id}", http.HandlerFunc(uh.ByID))
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
		mux.Handle("POST /chats/{chat_id}/messages", authMsg(http.HandlerFunc(mh.CreateInChat)))
		mux.Handle("GET /messages/{message_id}", authMsg(http.HandlerFunc(mh.Get)))
		mux.Handle("PATCH /messages/{message_id}", authMsg(http.HandlerFunc(mh.Patch)))
		mux.Handle("DELETE /messages/{message_id}", authMsg(http.HandlerFunc(mh.Delete)))
		mux.Handle("POST /messages/{message_id}/read", authMsg(http.HandlerFunc(mh.MarkRead)))
	}

	if deps.WS != nil {
		mux.Handle("GET /ws/connect", deps.WS)
	}
}

func healthHandler(_ *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
