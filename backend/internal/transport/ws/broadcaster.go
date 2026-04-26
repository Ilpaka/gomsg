package ws

import (
	"context"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
	redistore "goflow/backend/internal/repository/redis"
)

// Broadcaster fans out websocket envelopes to all members of a chat.
// When pub is set, PublishToChat publishes to Redis for multi-instance relay; a local
// subscriber (StartRedisRelay) delivers payloads to this process Hub. When pub is nil,
// delivery stays in-process only.
type Broadcaster struct {
	hub   *Hub
	chats repository.ChatRepository
	pub   *redistore.PubSubRepository
}

func NewBroadcaster(hub *Hub, chats repository.ChatRepository, pub *redistore.PubSubRepository) *Broadcaster {
	return &Broadcaster{hub: hub, chats: chats, pub: pub}
}

// PublishToChat marshals one envelope and either publishes to Redis or delivers locally.
func (b *Broadcaster) PublishToChat(ctx context.Context, chatID domain.ID, event string, data any, meta any) error {
	if b == nil || b.hub == nil || b.chats == nil {
		return nil
	}
	payload, err := MarshalEnvelope(event, data, meta)
	if err != nil {
		return err
	}
	if b.pub != nil {
		return b.pub.PublishChatEvent(ctx, string(chatID), payload)
	}
	return b.deliverPayload(ctx, chatID, payload)
}

// DeliverEnvelopeBytes sends a pre-serialized WS envelope JSON to all chat members (local hub only).
func (b *Broadcaster) DeliverEnvelopeBytes(ctx context.Context, chatID domain.ID, payload []byte) error {
	return b.deliverPayload(ctx, chatID, payload)
}

func (b *Broadcaster) deliverPayload(ctx context.Context, chatID domain.ID, payload []byte) error {
	if b == nil || b.hub == nil || b.chats == nil {
		return nil
	}
	members, err := b.chats.GetChatMembers(ctx, chatID)
	if err != nil {
		return err
	}
	for _, m := range members {
		b.hub.SendToUser(m.UserID, payload)
	}
	return nil
}

// StartRedisRelay subscribes to chat channels and forwards payloads to the local Hub.
// Run in a dedicated goroutine; blocks until ctx is cancelled.
func (b *Broadcaster) StartRedisRelay(ctx context.Context) error {
	if b == nil || b.pub == nil {
		return nil
	}
	return b.pub.SubscribeChatEvents(ctx, func(ctx context.Context, chatID string, payload []byte) error {
		return b.deliverPayload(ctx, domain.ID(chatID), payload)
	})
}
