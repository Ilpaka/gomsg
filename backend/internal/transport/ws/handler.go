package ws

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/observability/metrics"
)

// TicketConsumer resolves a one-time WS connect ticket to a user id.
type TicketConsumer interface {
	Consume(ctx context.Context, ticket string) (domain.ID, error)
}

// Handler upgrades HTTP to WebSocket using a short-lived ticket (not JWT in query).
type Handler struct {
	hub      *Hub
	proc     EventProcessor
	tickets  TicketConsumer
	allowed  map[string]struct{}
	log      *slog.Logger
	presence PresenceNotifier
	met      *metrics.M
}

func NewHandler(hub *Hub, proc EventProcessor, tickets TicketConsumer, allowedOrigins []string, log *slog.Logger, presence PresenceNotifier, met *metrics.M) *Handler {
	m := make(map[string]struct{})
	for _, o := range allowedOrigins {
		if t := strings.TrimSpace(o); t != "" {
			m[t] = struct{}{}
		}
	}
	return &Handler{hub: hub, proc: proc, tickets: tickets, allowed: m, log: log, presence: presence, met: met}
}

func (h *Handler) originOK(origin string) bool {
	if origin == "" {
		return true
	}
	if len(h.allowed) == 0 {
		return false
	}
	_, ok := h.allowed[origin]
	return ok
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.originOK(strings.TrimSpace(r.Header.Get("Origin"))) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if h.tickets == nil {
		http.Error(w, "ws not configured", http.StatusServiceUnavailable)
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		http.Error(w, "missing ticket", http.StatusUnauthorized)
		return
	}
	uid, err := h.tickets.Consume(r.Context(), ticket)
	if err != nil || uid == "" {
		http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
		return
	}

	up := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(*http.Request) bool { return true },
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		if h.met != nil {
			h.met.WSErrors.WithLabelValues("upgrade").Inc()
		}
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

	client := newClient(h.hub, conn, uid, r.Context(), onDisconnect, h.met)
	h.hub.Register(client)
	go client.writePump()
	client.readPump(h.proc)
}
