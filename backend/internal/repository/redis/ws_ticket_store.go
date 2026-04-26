package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"goflow/backend/internal/domain"
)

const wsTicketKeyPrefix = "goflow:ws:ticket:"

// WSTicketStore issues one-time WebSocket connect tickets in Redis.
type WSTicketStore struct {
	rdb *goredis.Client
}

func NewWSTicketStore(rdb *goredis.Client) *WSTicketStore {
	return &WSTicketStore{rdb: rdb}
}

func (s *WSTicketStore) Issue(ctx context.Context, userID domain.ID, ttl time.Duration) (string, error) {
	if s == nil || s.rdb == nil {
		return "", fmt.Errorf("ws ticket: nil client")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(raw[:])
	key := wsTicketKeyPrefix + ticket
	ok, err := s.rdb.SetNX(ctx, key, string(userID), ttl).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("ws ticket: collision")
	}
	return ticket, nil
}

// Consume validates a ticket, returns the user id, and deletes the key (one-time use).
func (s *WSTicketStore) Consume(ctx context.Context, ticket string) (domain.ID, error) {
	if s == nil || s.rdb == nil {
		return "", fmt.Errorf("ws ticket: nil client")
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return "", fmt.Errorf("ws ticket: empty")
	}
	key := wsTicketKeyPrefix + ticket
	val, err := s.rdb.GetDel(ctx, key).Result()
	if err == goredis.Nil {
		return "", fmt.Errorf("invalid or expired ticket")
	}
	if err != nil {
		return "", err
	}
	uid := domain.ID(strings.TrimSpace(val))
	if uid == "" {
		return "", fmt.Errorf("ws ticket: invalid payload")
	}
	return uid, nil
}
