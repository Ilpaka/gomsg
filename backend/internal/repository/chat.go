package repository

import (
	"context"
	"time"

	"goflow/backend/internal/domain"
)

// AddChatMember is a row for bulk insert into chat_members.
type AddChatMember struct {
	UserID domain.ID
	Role   domain.ChatMemberRole
}

// ChatListRow is one chat row for the authenticated user with last message preview and unread count.
type ChatListRow struct {
	Chat              domain.Chat
	LastPreviewText   *string
	LastPreviewType   *domain.MessageType
	LastPreviewSender *domain.ID
	LastPreviewAt     *time.Time
	UnreadCount       int64
}

// ChatRepository persists chats and membership.
type ChatRepository interface {
	CreateChat(ctx context.Context, c *domain.Chat) error
	CreateMember(ctx context.Context, m *domain.ChatMember) error
	GetByID(ctx context.Context, id domain.ID) (*domain.Chat, error)
	GetDirectByKey(ctx context.Context, directKey string) (*domain.Chat, error)
	GetUserChats(ctx context.Context, userID domain.ID, page Page) ([]domain.Chat, error)
	ListUserChatsSummary(ctx context.Context, userID domain.ID, page Page) ([]ChatListRow, error)
	GetChatMembers(ctx context.Context, chatID domain.ID) ([]domain.ChatMember, error)
	FindMembership(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error)
	AddMembers(ctx context.Context, chatID domain.ID, members []AddChatMember) error
	RemoveMember(ctx context.Context, chatID, userID domain.ID) error
	// CreateGroupWithMembers creates a group chat, adds creator as owner, then other users as members (transactional).
	CreateGroupWithMembers(ctx context.Context, title string, avatarURL *string, creator domain.ID, otherUserIDs []domain.ID) (*domain.Chat, error)
	UpdateLastMessage(ctx context.Context, chatID, messageID domain.ID, at time.Time) error
	UpdateMemberRead(ctx context.Context, chatID, userID, readUpToMessageID domain.ID) error
}
