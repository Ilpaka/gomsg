package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// ChatRepository implements repository.ChatRepository using pgx.
type ChatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool}
}

var _ repository.ChatRepository = (*ChatRepository)(nil)

func (r *ChatRepository) CreateChat(ctx context.Context, c *domain.Chat) error {
	const q = `
INSERT INTO chats (type, title, avatar_url, created_by, direct_key)
VALUES ($1, $2, $3, $4::uuid, $5)
RETURNING id::text, type, title, avatar_url, created_by::text, created_at, updated_at,
          last_message_id::text, last_message_at, is_deleted, direct_key`

	var createdBy *string
	if c.CreatedBy != nil {
		s := string(*c.CreatedBy)
		createdBy = &s
	}

	row := r.pool.QueryRow(ctx, q,
		string(c.Type),
		c.Title,
		c.AvatarURL,
		createdBy,
		c.DirectKey,
	)
	out, err := scanChat(row)
	if err != nil {
		return err
	}
	*c = out
	return nil
}

func (r *ChatRepository) CreateMember(ctx context.Context, m *domain.ChatMember) error {
	const q = `
INSERT INTO chat_members (chat_id, user_id, role, last_read_message_id, last_read_at, is_muted, is_archived, is_pinned)
VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5, $6, $7, $8)
RETURNING chat_id::text, user_id::text, role, joined_at, last_read_message_id::text, last_read_at, is_muted, is_archived, is_pinned`

	var lastRead *string
	if m.LastReadMessageID != nil {
		s := string(*m.LastReadMessageID)
		lastRead = &s
	}

	row := r.pool.QueryRow(ctx, q,
		string(m.ChatID),
		string(m.UserID),
		string(m.Role),
		lastRead,
		m.LastReadAt,
		m.IsMuted,
		m.IsArchived,
		m.IsPinned,
	)
	out, err := scanChatMember(row)
	if err != nil {
		return err
	}
	*m = out
	return nil
}

func (r *ChatRepository) GetByID(ctx context.Context, id domain.ID) (*domain.Chat, error) {
	const q = `
SELECT id::text, type, title, avatar_url, created_by::text, created_at, updated_at,
       last_message_id::text, last_message_at, is_deleted, direct_key
FROM chats
WHERE id = $1::uuid`

	row := r.pool.QueryRow(ctx, q, string(id))
	c, err := scanChat(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &c, nil
}

func (r *ChatRepository) GetDirectByKey(ctx context.Context, directKey string) (*domain.Chat, error) {
	const q = `
SELECT id::text, type, title, avatar_url, created_by::text, created_at, updated_at,
       last_message_id::text, last_message_at, is_deleted, direct_key
FROM chats
WHERE direct_key = $1
  AND type = 'direct'
  AND is_deleted = false`

	row := r.pool.QueryRow(ctx, q, directKey)
	c, err := scanChat(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &c, nil
}

func (r *ChatRepository) GetUserChats(ctx context.Context, userID domain.ID, page repository.Page) ([]domain.Chat, error) {
	limit, offset := normPage(page)
	const q = `
SELECT c.id::text, c.type, c.title, c.avatar_url, c.created_by::text, c.created_at, c.updated_at,
       c.last_message_id::text, c.last_message_at, c.is_deleted, c.direct_key
FROM chats c
INNER JOIN chat_members m ON m.chat_id = c.id
WHERE m.user_id = $1::uuid
  AND c.is_deleted = false
ORDER BY c.last_message_at DESC NULLS LAST, c.updated_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, string(userID), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Chat
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ChatRepository) ListUserChatsSummary(ctx context.Context, userID domain.ID, page repository.Page) ([]repository.ChatListRow, error) {
	limit, offset := normPage(page)
	const q = `
SELECT
  c.id::text, c.type, c.title, c.avatar_url, c.created_by::text, c.created_at, c.updated_at,
  c.last_message_id::text, c.last_message_at, c.is_deleted, c.direct_key,
  lm.text AS last_preview_text,
  lm.type AS last_preview_type,
  lm.sender_id::text AS last_preview_sender_id,
  lm.created_at AS last_preview_at,
  (
    SELECT COUNT(*)::bigint
    FROM messages mu
    WHERE mu.chat_id = c.id
      AND mu.deleted_at IS NULL
      AND (
        mem.last_read_message_id IS NULL
        OR EXISTS (
          SELECT 1
          FROM messages anchor
          WHERE anchor.id = mem.last_read_message_id
            AND anchor.chat_id = c.id
            AND (
              mu.created_at > anchor.created_at
              OR (mu.created_at = anchor.created_at AND mu.id > anchor.id)
            )
        )
      )
  ) AS unread_count
FROM chats c
INNER JOIN chat_members mem ON mem.chat_id = c.id AND mem.user_id = $1::uuid
LEFT JOIN messages lm ON lm.id = c.last_message_id AND lm.deleted_at IS NULL
WHERE c.is_deleted = false
ORDER BY c.last_message_at DESC NULLS LAST, c.updated_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, string(userID), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repository.ChatListRow
	for rows.Next() {
		row, err := scanChatListRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ChatRepository) FindMembership(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error) {
	const q = `
SELECT chat_id::text, user_id::text, role, joined_at, last_read_message_id::text, last_read_at,
       is_muted, is_archived, is_pinned
FROM chat_members
WHERE chat_id = $1::uuid AND user_id = $2::uuid`

	row := r.pool.QueryRow(ctx, q, string(chatID), string(userID))
	m, err := scanChatMember(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &m, nil
}

func (r *ChatRepository) GetChatMembers(ctx context.Context, chatID domain.ID) ([]domain.ChatMember, error) {
	const q = `
SELECT chat_id::text, user_id::text, role, joined_at, last_read_message_id::text, last_read_at,
       is_muted, is_archived, is_pinned
FROM chat_members
WHERE chat_id = $1::uuid
ORDER BY joined_at ASC`

	rows, err := r.pool.Query(ctx, q, string(chatID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ChatMember
	for rows.Next() {
		m, err := scanChatMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ChatRepository) AddMembers(ctx context.Context, chatID domain.ID, members []repository.AddChatMember) error {
	if len(members) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO chat_members (chat_id, user_id, role) VALUES `)
	args := make([]any, 0, 1+len(members)*2)
	args = append(args, string(chatID))
	argPos := 2
	for i, m := range members {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("($1::uuid, $%d::uuid, $%d)", argPos, argPos+1))
		args = append(args, string(m.UserID), string(m.Role))
		argPos += 2
	}
	sb.WriteString(` ON CONFLICT (chat_id, user_id) DO NOTHING`)

	_, err := r.pool.Exec(ctx, sb.String(), args...)
	return err
}

func (r *ChatRepository) RemoveMember(ctx context.Context, chatID, userID domain.ID) error {
	const q = `DELETE FROM chat_members WHERE chat_id = $1::uuid AND user_id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, string(chatID), string(userID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *ChatRepository) CreateGroupWithMembers(ctx context.Context, title string, avatarURL *string, creator domain.ID, otherUserIDs []domain.ID) (*domain.Chat, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertChat = `
INSERT INTO chats (type, title, avatar_url, created_by, direct_key)
VALUES ('group', $1, $2, $3::uuid, NULL)
RETURNING id::text, type, title, avatar_url, created_by::text, created_at, updated_at,
          last_message_id::text, last_message_at, is_deleted, direct_key`

	row := tx.QueryRow(ctx, insertChat, title, avatarURL, string(creator))
	c, err := scanChat(row)
	if err != nil {
		return nil, err
	}

	const insOwner = `
INSERT INTO chat_members (chat_id, user_id, role, last_read_message_id, last_read_at, is_muted, is_archived, is_pinned)
VALUES ($1::uuid, $2::uuid, 'owner', NULL, NULL, false, false, false)`
	if _, err := tx.Exec(ctx, insOwner, string(c.ID), string(creator)); err != nil {
		return nil, err
	}

	seen := map[domain.ID]struct{}{creator: {}}
	var toAdd []domain.ID
	for _, uid := range otherUserIDs {
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		toAdd = append(toAdd, uid)
	}
	if len(toAdd) > 0 {
		var sb strings.Builder
		sb.WriteString(`INSERT INTO chat_members (chat_id, user_id, role) VALUES `)
		args := []any{string(c.ID)}
		argPos := 2
		for i, uid := range toAdd {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("($1::uuid, $%d::uuid, 'member')", argPos))
			args = append(args, string(uid))
			argPos++
		}
		sb.WriteString(` ON CONFLICT (chat_id, user_id) DO NOTHING`)
		if _, err := tx.Exec(ctx, sb.String(), args...); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ChatRepository) UpdateLastMessage(ctx context.Context, chatID, messageID domain.ID, at time.Time) error {
	const q = `
UPDATE chats
SET last_message_id = $2::uuid,
    last_message_at = $3,
    updated_at = now()
WHERE id = $1::uuid`

	tag, err := r.pool.Exec(ctx, q, string(chatID), string(messageID), at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *ChatRepository) UpdateMemberRead(ctx context.Context, chatID, userID, readUpToMessageID domain.ID) error {
	const q = `
UPDATE chat_members
SET last_read_message_id = $3::uuid,
    last_read_at = now()
WHERE chat_id = $1::uuid AND user_id = $2::uuid`

	tag, err := r.pool.Exec(ctx, q, string(chatID), string(userID), string(readUpToMessageID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

type chatScanner interface {
	Scan(dest ...any) error
}

func scanChatListRow(row chatScanner) (repository.ChatListRow, error) {
	var c domain.Chat
	var id string
	var createdBy, lastMsg, directKey *string
	var typ string
	var previewText, previewType, previewSender *string
	var previewAt *time.Time
	var unread int64
	err := row.Scan(
		&id,
		&typ,
		&c.Title,
		&c.AvatarURL,
		&createdBy,
		&c.CreatedAt,
		&c.UpdatedAt,
		&lastMsg,
		&c.LastMessageAt,
		&c.IsDeleted,
		&directKey,
		&previewText,
		&previewType,
		&previewSender,
		&previewAt,
		&unread,
	)
	if err != nil {
		return repository.ChatListRow{}, err
	}
	c.Type = domain.ChatType(typ)
	c.ID = domain.ID(id)
	if createdBy != nil {
		cb := domain.ID(*createdBy)
		c.CreatedBy = &cb
	}
	if lastMsg != nil {
		lm := domain.ID(*lastMsg)
		c.LastMessageID = &lm
	}
	if directKey != nil {
		c.DirectKey = directKey
	}
	var lastType *domain.MessageType
	if previewType != nil {
		t := domain.MessageType(*previewType)
		lastType = &t
	}
	var lastSender *domain.ID
	if previewSender != nil {
		s := domain.ID(*previewSender)
		lastSender = &s
	}
	return repository.ChatListRow{
		Chat:              c,
		LastPreviewText:   previewText,
		LastPreviewType:   lastType,
		LastPreviewSender: lastSender,
		LastPreviewAt:     previewAt,
		UnreadCount:       unread,
	}, nil
}

func scanChat(row chatScanner) (domain.Chat, error) {
	var c domain.Chat
	var id string
	var createdBy, lastMsg, directKey *string
	var typ string
	err := row.Scan(
		&id,
		&typ,
		&c.Title,
		&c.AvatarURL,
		&createdBy,
		&c.CreatedAt,
		&c.UpdatedAt,
		&lastMsg,
		&c.LastMessageAt,
		&c.IsDeleted,
		&directKey,
	)
	if err != nil {
		return domain.Chat{}, err
	}
	c.Type = domain.ChatType(typ)
	c.ID = domain.ID(id)
	if createdBy != nil {
		cb := domain.ID(*createdBy)
		c.CreatedBy = &cb
	}
	if lastMsg != nil {
		lm := domain.ID(*lastMsg)
		c.LastMessageID = &lm
	}
	if directKey != nil {
		c.DirectKey = directKey
	}
	return c, nil
}

func scanChatMember(row chatScanner) (domain.ChatMember, error) {
	var m domain.ChatMember
	var chatID, userID string
	var lastRead *string
	var role string
	err := row.Scan(
		&chatID,
		&userID,
		&role,
		&m.JoinedAt,
		&lastRead,
		&m.LastReadAt,
		&m.IsMuted,
		&m.IsArchived,
		&m.IsPinned,
	)
	if err != nil {
		return domain.ChatMember{}, err
	}
	m.Role = domain.ChatMemberRole(role)
	m.ChatID = domain.ID(chatID)
	m.UserID = domain.ID(userID)
	if lastRead != nil {
		lr := domain.ID(*lastRead)
		m.LastReadMessageID = &lr
	}
	return m, nil
}
