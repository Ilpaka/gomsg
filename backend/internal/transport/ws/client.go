package ws

import (
	"context"
	"time"

	"github.com/gorilla/websocket"

	"goflow/backend/internal/domain"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 65536
)

// EventProcessor handles one inbound JSON text frame from a client.
type EventProcessor interface {
	HandleEvent(ctx context.Context, userID domain.ID, payload []byte) ([][]byte, error)
}

// Client is one websocket connection for a user.
type Client struct {
	hub          *Hub
	conn         *websocket.Conn
	send         chan []byte
	userID       domain.ID
	ctx          context.Context
	onDisconnect func(domain.ID)
}

func newClient(hub *Hub, conn *websocket.Conn, userID domain.ID, ctx context.Context, onDisconnect func(domain.ID)) *Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Client{
		hub:          hub,
		conn:         conn,
		send:         make(chan []byte, 256),
		userID:       userID,
		ctx:          ctx,
		onDisconnect: onDisconnect,
	}
}

func (c *Client) readPump(proc EventProcessor) {
	defer func() {
		if c.onDisconnect != nil {
			c.onDisconnect(c.userID)
		}
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		outs, err := proc.HandleEvent(c.ctx, c.userID, message)
		if err != nil {
			if b, mErr := MarshalEnvelope(EventError, map[string]any{
				"code":    "internal",
				"message": err.Error(),
			}, nil); mErr == nil {
				_ = c.writeDirect(b)
			}
			continue
		}
		for _, b := range outs {
			if b == nil {
				continue
			}
			select {
			case c.send <- b:
			default:
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) writeDirect(b []byte) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(websocket.TextMessage, b)
}
