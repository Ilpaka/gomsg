package redis

import (
	"context"
	"fmt"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

const chatChannelPrefix = "goflow:chat:"

// PubSubRepository publishes chat-scoped fanout payloads for multi-instance WebSocket relays.
// Payloads are opaque JSON bytes (envelopes); Redis is not a message store.
type PubSubRepository struct {
	rdb *goredis.Client
}

func NewPubSubRepository(rdb *goredis.Client) *PubSubRepository {
	return &PubSubRepository{rdb: rdb}
}

func chatChannel(chatID string) string {
	return chatChannelPrefix + chatID
}

// PublishChatEvent publishes a payload to all subscribers of the chat channel.
func (r *PubSubRepository) PublishChatEvent(ctx context.Context, chatID string, payload []byte) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("pubsub: nil client")
	}
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("pubsub: empty chat id")
	}
	return r.rdb.Publish(ctx, chatChannel(chatID), payload).Err()
}

// SubscribeChatEvents subscribes to all chat channels and invokes handler for each message.
// Blocks until ctx is cancelled or the subscription ends.
func (r *PubSubRepository) SubscribeChatEvents(ctx context.Context, handler func(ctx context.Context, chatID string, payload []byte) error) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("pubsub: nil client")
	}
	sub := r.rdb.PSubscribe(ctx, chatChannelPrefix+"*")
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if msg == nil {
				continue
			}
			chatID := strings.TrimPrefix(msg.Channel, chatChannelPrefix)
			if chatID == "" {
				continue
			}
			if err := handler(ctx, chatID, []byte(msg.Payload)); err != nil {
				return err
			}
		}
	}
}
