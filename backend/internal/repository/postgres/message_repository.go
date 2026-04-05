package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// MessageRepository implements repository.MessageRepository using pgx.
type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

var _ repository.MessageRepository = (*MessageRepository)(nil)

func (r *MessageRepository) Create(ctx context.Context, m *domain.Message) error {
	const q = `
INSERT INTO messages (chat_id, sender_id, type, text, reply_to_id)
VALUES ($1::uuid, $2::uuid, $3, $4, $5)
RETURNING id::text, chat_id::text, sender_id::text, type, text, reply_to_id::text,
          created_at, updated_at, deleted_at`

	var reply *string
	if m.ReplyToID != nil {
		s := string(*m.ReplyToID)
		reply = &s
	}

	row := r.pool.QueryRow(ctx, q,
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
	return nil
}

func (r *MessageRepository) GetByID(ctx context.Context, id domain.ID) (*domain.Message, error) {
	const q = `
SELECT id::text, chat_id::text, sender_id::text, type, text, reply_to_id::text,
       created_at, updated_at, deleted_at
FROM messages
WHERE id = $1::uuid`

	row := r.pool.QueryRow(ctx, q, string(id))
	m, err := scanMessage(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &m, nil
}

func (r *MessageRepository) GetChatMessages(ctx context.Context, chatID domain.ID, opts repository.MessageListOpts) ([]domain.Message, error) {
	limit := normMessageLimit(opts.Limit)

	const q = `
SELECT m.id::text, m.chat_id::text, m.sender_id::text, m.type, m.text, m.reply_to_id::text,
       m.created_at, m.updated_at, m.deleted_at
FROM messages m
WHERE m.chat_id = $1::uuid
  AND m.deleted_at IS NULL
  AND (
    (
      $2::uuid IS NOT NULL
      AND (m.created_at, m.id) < (
        SELECT anchor.created_at, anchor.id
        FROM messages anchor
        WHERE anchor.id = $2::uuid AND anchor.chat_id = $1::uuid
      )
    )
    OR (
      $2::uuid IS NULL
      AND $4::timestamptz IS NOT NULL
      AND m.created_at < $4::timestamptz
    )
    OR (
      $2::uuid IS NULL
      AND $4::timestamptz IS NULL
    )
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT $3`

	var beforeID any
	if opts.BeforeID != nil {
		beforeID = string(*opts.BeforeID)
	}
	var beforeTime any
	if opts.BeforeTime != nil {
		beforeTime = opts.BeforeTime.UTC()
	}

	rows, err := r.pool.Query(ctx, q, string(chatID), beforeID, limit, beforeTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MessageRepository) UpdateText(ctx context.Context, chatID, messageID domain.ID, text string) error {
	const q = `
UPDATE messages
SET text = $3,
    updated_at = now()
WHERE id = $2::uuid AND chat_id = $1::uuid AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, string(chatID), string(messageID), text)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *MessageRepository) SoftDelete(ctx context.Context, chatID, messageID domain.ID) error {
	const q = `
UPDATE messages
SET deleted_at = now(),
    updated_at = now()
WHERE id = $2::uuid AND chat_id = $1::uuid AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, string(chatID), string(messageID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *MessageRepository) CountUnreadAfter(ctx context.Context, chatID domain.ID, afterMessageID *domain.ID) (int64, error) {
	const q = `
SELECT COUNT(*)::bigint
FROM messages m
WHERE m.chat_id = $1::uuid
  AND m.deleted_at IS NULL
  AND (
    $2::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM messages anchor
      WHERE anchor.id = $2::uuid
        AND anchor.chat_id = $1::uuid
        AND (
          m.created_at > anchor.created_at
          OR (m.created_at = anchor.created_at AND m.id > anchor.id)
        )
    )
  )`

	var afterID any
	if afterMessageID != nil {
		afterID = string(*afterMessageID)
	}

	var n int64
	err := r.pool.QueryRow(ctx, q, string(chatID), afterID).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row messageScanner) (domain.Message, error) {
	var m domain.Message
	var id, chatID, senderID string
	var replyID *string
	var typ string
	err := row.Scan(
		&id,
		&chatID,
		&senderID,
		&typ,
		&m.Text,
		&replyID,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	m.Type = domain.MessageType(typ)
	m.ID = domain.ID(id)
	m.ChatID = domain.ID(chatID)
	m.SenderID = domain.ID(senderID)
	if replyID != nil {
		rid := domain.ID(*replyID)
		m.ReplyToID = &rid
	}
	return m, nil
}
