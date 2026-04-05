package ws

import (
	"context"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// Broadcaster fans out websocket envelopes to all members of a chat.
type Broadcaster struct {
	hub   *Hub
	chats repository.ChatRepository
}

func NewBroadcaster(hub *Hub, chats repository.ChatRepository) *Broadcaster {
	return &Broadcaster{hub: hub, chats: chats}
}

// PublishToChat marshals one envelope and delivers it to every member connection.
func (b *Broadcaster) PublishToChat(ctx context.Context, chatID domain.ID, event string, data any, meta any) error {
	if b == nil || b.hub == nil || b.chats == nil {
		return nil
	}
	payload, err := MarshalEnvelope(event, data, meta)
	if err != nil {
		return err
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
