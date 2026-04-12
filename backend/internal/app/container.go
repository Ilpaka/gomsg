package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"goflow/backend/internal/config"
	"goflow/backend/internal/repository"
	"goflow/backend/internal/repository/postgres"
	redistore "goflow/backend/internal/repository/redis"
)

// Container wires application dependencies.
type Container struct {
	Config *config.Config
	Logger *slog.Logger
	Pool   *pgxpool.Pool
	Redis  *goredis.Client

	Users    repository.UserRepository
	Chats    repository.ChatRepository
	Messages repository.MessageRepository
	Sessions repository.SessionRepository

	Presence *redistore.PresenceRepository
	Typing   *redistore.TypingRepository
	PubSub   *redistore.PubSubRepository
}

// NewContainer returns a container. When pool is non-nil, PostgreSQL repositories are constructed.
// Redis client is created when cfg.Redis.Addr is set (required by config validation).
func NewContainer(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool) (*Container, error) {
	c := &Container{
		Config: cfg,
		Logger: log,
		Pool:   pool,
	}
	if pool != nil {
		c.Users = postgres.NewUserRepository(pool)
		c.Chats = postgres.NewChatRepository(pool)
		c.Messages = postgres.NewMessageRepository(pool)
		c.Sessions = postgres.NewSessionRepository(pool)
	}

	addr := strings.TrimSpace(cfg.Redis.Addr)
	if addr != "" {
		rdb := goredis.NewClient(&goredis.Options{Addr: addr})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		c.Redis = rdb
		c.Presence = redistore.NewPresenceRepository(rdb)
		c.Typing = redistore.NewTypingRepository(rdb)
		c.PubSub = redistore.NewPubSubRepository(rdb)
	}

	return c, nil
}

// Close releases external resources owned by the container.
func (c *Container) Close() {
	if c == nil || c.Redis == nil {
		return
	}
	_ = c.Redis.Close()
}
