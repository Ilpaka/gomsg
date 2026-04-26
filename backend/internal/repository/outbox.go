package repository

import (
	"context"
	"encoding/json"
	"time"

	"goflow/backend/internal/domain"
)

// OutboxEnqueue is written atomically with the domain mutation.
type OutboxEnqueue struct {
	EventID       domain.ID
	EventType     string
	AggregateType string
	AggregateID   string
	ChatID        *domain.ID
	OccurredAt    time.Time
	Version       int
	Payload       json.RawMessage
}

// OutboxRow is a persisted outbox record waiting for publication.
type OutboxRow struct {
	ID            int64
	EventID       string
	EventType     string
	AggregateType string
	AggregateID   string
	ChatID        *string
	OccurredAt    time.Time
	Version       int
	Payload       json.RawMessage
}

// OutboxRepository reads and marks outbox rows for the relay worker.
type OutboxRepository interface {
	FetchPending(ctx context.Context, limit int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, id int64) error
}

// MessageWriter performs message mutations together with an outbox row in one transaction.
type MessageWriter interface {
	CreateMessageWithOutbox(ctx context.Context, m *domain.Message, ob OutboxEnqueue) error
	UpdateMessageTextWithOutbox(ctx context.Context, chatID, messageID domain.ID, text string, ob OutboxEnqueue) error
	SoftDeleteMessageWithOutbox(ctx context.Context, chatID, messageID domain.ID, ob OutboxEnqueue) error
	UpdateMemberReadWithOutbox(ctx context.Context, chatID, userID, readUpToMessageID domain.ID, ob OutboxEnqueue) error
}
