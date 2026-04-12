package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"goflow/backend/internal/domain"
)

const typingKeyPrefix = "goflow:typing:"

// DefaultTypingTTL is how long a typing indicator lives without refresh.
const DefaultTypingTTL = 8 * time.Second

// TypingRepository stores ephemeral typing indicators per (chat, user).
type TypingRepository struct {
	rdb *goredis.Client
	ttl time.Duration
}

func NewTypingRepository(rdb *goredis.Client) *TypingRepository {
	return &TypingRepository{rdb: rdb, ttl: DefaultTypingTTL}
}

func typingKey(chatID, userID domain.ID) string {
	return typingKeyPrefix + string(chatID) + ":" + string(userID)
}

// StartTyping sets a short-lived key for the user typing in the chat.
func (r *TypingRepository) StartTyping(ctx context.Context, chatID, userID domain.ID) error {
	if r == nil || r.rdb == nil {
		return nil
	}
	if chatID == "" || userID == "" {
		return fmt.Errorf("typing: empty chat or user id")
	}
	return r.rdb.Set(ctx, typingKey(chatID, userID), "1", r.ttl).Err()
}

// StopTyping removes the typing indicator immediately.
func (r *TypingRepository) StopTyping(ctx context.Context, chatID, userID domain.ID) error {
	if r == nil || r.rdb == nil {
		return nil
	}
	if chatID == "" || userID == "" {
		return nil
	}
	return r.rdb.Del(ctx, typingKey(chatID, userID)).Err()
}
