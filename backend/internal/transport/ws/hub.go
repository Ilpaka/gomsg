package ws

import (
	"sync"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/observability/metrics"
)

// Hub tracks all active websocket clients keyed by user id (multiple connections per user).
type Hub struct {
	mu      sync.RWMutex
	clients map[domain.ID]map[*Client]struct{}
	met     *metrics.M
}

func NewHub(m *metrics.M) *Hub {
	return &Hub{clients: make(map[domain.ID]map[*Client]struct{}), met: m}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[c.userID]
	if set == nil {
		set = make(map[*Client]struct{})
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
	if h.met != nil {
		h.met.WSActive.Inc()
	}
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.clients[c.userID]
	if !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.clients, c.userID)
	}
	if h.met != nil {
		h.met.WSActive.Dec()
	}
}

// SendToUser delivers the same payload to every connection of the user (copy per client).
func (h *Hub) SendToUser(userID domain.ID, payload []byte) {
	h.mu.RLock()
	set := h.clients[userID]
	// copy slice of clients to minimize lock while sending
	clients := make([]*Client, 0, len(set))
	for c := range set {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		cp := append([]byte(nil), payload...)
		select {
		case c.send <- cp:
		default:
			// drop if client is slow; connection will catch up on next events
		}
	}
}

// CloseAll closes every active websocket connection (used during HTTP server shutdown).
func (h *Hub) CloseAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	var clients []*Client
	for _, set := range h.clients {
		for c := range set {
			clients = append(clients, c)
		}
	}
	h.mu.Unlock()

	for _, c := range clients {
		if c != nil {
			_ = c.closeForShutdown()
		}
	}
}
