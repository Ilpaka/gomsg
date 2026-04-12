package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"goflow/backend/internal/domain"
)

const presenceKeyPrefix = "goflow:presence:user:"

// DefaultPresenceTTL is how long a user stays "online" without a heartbeat refresh.
const DefaultPresenceTTL = 45 * time.Second

// PresenceRepository stores ephemeral online state (not source of truth for users).
type PresenceRepository struct {
	rdb *goredis.Client
	ttl time.Duration
}

func NewPresenceRepository(rdb *goredis.Client) *PresenceRepository {
	return &PresenceRepository{rdb: rdb, ttl: DefaultPresenceTTL}
}

func presenceKey(userID domain.ID) string {
	return presenceKeyPrefix + string(userID)
}

// SetOnline marks the user online and (re)starts the heartbeat TTL.
func (r *PresenceRepository) SetOnline(ctx context.Context, userID domain.ID) error {
	if r == nil || r.rdb == nil {
		return nil
	}
	if userID == "" {
		return fmt.Errorf("presence: empty user id")
	}
	return r.rdb.Set(ctx, presenceKey(userID), "1", r.ttl).Err()
}

// SetOffline removes online state immediately.
func (r *PresenceRepository) SetOffline(ctx context.Context, userID domain.ID) error {
	if r == nil || r.rdb == nil {
		return nil
	}
	if userID == "" {
		return nil
	}
	return r.rdb.Del(ctx, presenceKey(userID)).Err()
}

// IsOnline reports whether the presence key exists (within TTL window).
func (r *PresenceRepository) IsOnline(ctx context.Context, userID domain.ID) (bool, error) {
	if r == nil || r.rdb == nil {
		return false, nil
	}
	if userID == "" {
		return false, nil
	}
	n, err := r.rdb.Exists(ctx, presenceKey(userID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
