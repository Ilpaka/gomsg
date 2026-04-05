package repository

import (
	"context"
	"time"

	"goflow/backend/internal/domain"
)

// MessageListOpts controls listing messages in a chat (newest first).
// BeforeID, when set, returns messages strictly older than that message (same chat).
// BeforeTime, when set and BeforeID is nil, returns messages with created_at strictly less than BeforeTime.
type MessageListOpts struct {
	Limit      int
	BeforeID   *domain.ID
	BeforeTime *time.Time
}

// MessageRepository persists chat messages.
type MessageRepository interface {
	Create(ctx context.Context, m *domain.Message) error
	GetByID(ctx context.Context, id domain.ID) (*domain.Message, error)
	GetChatMessages(ctx context.Context, chatID domain.ID, opts MessageListOpts) ([]domain.Message, error)
	UpdateText(ctx context.Context, chatID, messageID domain.ID, text string) error
	SoftDelete(ctx context.Context, chatID, messageID domain.ID) error
	CountUnreadAfter(ctx context.Context, chatID domain.ID, afterMessageID *domain.ID) (int64, error)
}
