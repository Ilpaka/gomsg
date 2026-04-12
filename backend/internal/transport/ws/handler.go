package ws

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/pkg/auth"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler upgrades HTTP to WebSocket and binds a Client to the Hub.
type Handler struct {
	hub      *Hub
	proc     EventProcessor
	secret   []byte
	log      *slog.Logger
	presence PresenceNotifier
}

func NewHandler(hub *Hub, proc EventProcessor, jwtSecret []byte, log *slog.Logger, presence PresenceNotifier) *Handler {
	return &Handler{hub: hub, proc: proc, secret: jwtSecret, log: log, presence: presence}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		http.Error(w, "missing access_token", http.StatusUnauthorized)
		return
	}
	claims, err := auth.ParseAccessToken(h.secret, token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	uid := domain.ID(strings.TrimSpace(claims.UserID))
	if uid == "" {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.log != nil {
			h.log.Warn("ws upgrade failed", "err", err)
		}
		return
	}

	if h.presence != nil {
		h.presence.Connected(r.Context(), uid)
	}

	onDisconnect := func(id domain.ID) {
		if h.presence != nil {
			h.presence.Disconnected(context.Background(), id)
		}
	}

	client := newClient(h.hub, conn, uid, r.Context(), onDisconnect)
	h.hub.Register(client)
	go client.writePump()
	client.readPump(h.proc)
}
