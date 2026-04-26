package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/observability/metrics"
	"goflow/backend/internal/repository"
)

// MessageWriter performs transactional message writes + outbox enqueue.
type MessageWriter struct {
	pool *pgxpool.Pool
	obs  *metrics.M
}

func NewMessageWriter(pool *pgxpool.Pool, obs *metrics.M) *MessageWriter {
	return &MessageWriter{pool: pool, obs: obs}
}

var _ repository.MessageWriter = (*MessageWriter)(nil)

func (w *MessageWriter) CreateMessageWithOutbox(ctx context.Context, m *domain.Message, ob repository.OutboxEnqueue) error {
	if w == nil || w.pool == nil {
		return fmt.Errorf("message writer: nil")
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insMsg = `
INSERT INTO messages (chat_id, sender_id, type, text, reply_to_id)
VALUES ($1::uuid, $2::uuid, $3, $4, $5)
RETURNING id::text, chat_id::text, sender_id::text, type, text, reply_to_id::text,
          created_at, updated_at, deleted_at`
	var reply *string
	if m.ReplyToID != nil {
		s := string(*m.ReplyToID)
		reply = &s
	}
	row := tx.QueryRow(ctx, insMsg,
		string(m.ChatID),
		string(m.SenderID),
		string(m.Type),
		m.Text,
		reply,
	)
	out, err := scanMessage(row)
	if err != nil {
		return err
	}
	*m = out

	ob2 := ob
	ob2.AggregateID = string(m.ID)
	ob2.OccurredAt = m.CreatedAt.UTC()
	if len(ob2.Payload) == 0 {
		raw, err := encodeMessageWireJSON(m)
		if err != nil {
			return err
		}
		ob2.Payload = raw
	}

	const updChat = `
UPDATE chats
SET last_message_id = $2::uuid,
    last_message_at = $3,
    updated_at = now()
WHERE id = $1::uuid`
	tag, err := tx.Exec(ctx, updChat, string(m.ChatID), string(m.ID), m.CreatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	if err := insertOutboxTx(ctx, tx, ob2); err != nil {
		return err
	}
	return w.commitOutboxObs(ctx, tx)
}

func (w *MessageWriter) UpdateMessageTextWithOutbox(ctx context.Context, chatID, messageID domain.ID, text string, ob repository.OutboxEnqueue) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const upd = `
UPDATE messages
SET text = $3,
    updated_at = now()
WHERE id = $2::uuid AND chat_id = $1::uuid AND deleted_at IS NULL`
	tag, err := tx.Exec(ctx, upd, string(chatID), string(messageID), text)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	row := tx.QueryRow(ctx, `
SELECT id::text, chat_id::text, sender_id::text, type, text, reply_to_id::text,
       created_at, updated_at, deleted_at
FROM messages WHERE id = $1::uuid AND chat_id = $2::uuid`, string(messageID), string(chatID))
	mUpd, err := scanMessage(row)
	if err != nil {
		return err
	}
	ob2 := ob
	if len(ob2.Payload) == 0 {
		raw, err := encodeMessageWireJSON(&mUpd)
		if err != nil {
			return err
		}
		ob2.Payload = raw
	}
	ob2.AggregateID = string(messageID)
	ob2.OccurredAt = mUpd.UpdatedAt.UTC()

	if err := insertOutboxTx(ctx, tx, ob2); err != nil {
		return err
	}
	return w.commitOutboxObs(ctx, tx)
}

func (w *MessageWriter) SoftDeleteMessageWithOutbox(ctx context.Context, chatID, messageID domain.ID, ob repository.OutboxEnqueue) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const upd = `
UPDATE messages
SET deleted_at = now(),
    updated_at = now()
WHERE id = $2::uuid AND chat_id = $1::uuid AND deleted_at IS NULL`
	tag, err := tx.Exec(ctx, upd, string(chatID), string(messageID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	if err := insertOutboxTx(ctx, tx, ob); err != nil {
		return err
	}
	return w.commitOutboxObs(ctx, tx)
}

func (w *MessageWriter) UpdateMemberReadWithOutbox(ctx context.Context, chatID, userID, readUpToMessageID domain.ID, ob repository.OutboxEnqueue) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const upd = `
UPDATE chat_members
SET last_read_message_id = $3::uuid,
    last_read_at = now()
WHERE chat_id = $1::uuid AND user_id = $2::uuid`
	tag, err := tx.Exec(ctx, upd, string(chatID), string(userID), string(readUpToMessageID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	if err := insertOutboxTx(ctx, tx, ob); err != nil {
		return err
	}
	return w.commitOutboxObs(ctx, tx)
}

func (w *MessageWriter) commitOutboxObs(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if w.obs != nil {
		w.obs.OutboxCreated.Inc()
	}
	return nil
}

func insertOutboxTx(ctx context.Context, tx pgx.Tx, ob repository.OutboxEnqueue) error {
	var chat any
	if ob.ChatID != nil {
		chat = string(*ob.ChatID)
	}
	payload := []byte(ob.Payload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	const q = `
INSERT INTO outbox_events (event_id, event_type, aggregate_type, aggregate_id, chat_id, occurred_at, version, payload)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::jsonb)`
	_, err := tx.Exec(ctx, q,
		string(ob.EventID),
		ob.EventType,
		ob.AggregateType,
		ob.AggregateID,
		chat,
		ob.OccurredAt.UTC(),
		ob.Version,
		payload,
	)
	return err
}

func encodeMessageWireJSON(m *domain.Message) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	var reply *string
	if m.ReplyToID != nil {
		s := string(*m.ReplyToID)
		reply = &s
	}
	w := struct {
		ID        string    `json:"id"`
		ChatID    string    `json:"chat_id"`
		SenderID  string    `json:"sender_id"`
		Type      string    `json:"type"`
		Text      *string   `json:"text,omitempty"`
		ReplyToID *string   `json:"reply_to_id,omitempty"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}{
		ID:        string(m.ID),
		ChatID:    string(m.ChatID),
		SenderID:  string(m.SenderID),
		Type:      string(m.Type),
		Text:      m.Text,
		ReplyToID: reply,
		CreatedAt: m.CreatedAt.UTC(),
		UpdatedAt: m.UpdatedAt.UTC(),
	}
	return json.Marshal(w)
}
