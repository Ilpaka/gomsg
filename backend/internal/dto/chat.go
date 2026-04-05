package dto

import "time"

// CreateDirectChatRequest is the body for POST /chats/direct.
type CreateDirectChatRequest struct {
	UserID string `json:"user_id"`
}

// CreateGroupChatRequest is the body for POST /chats/group.
type CreateGroupChatRequest struct {
	Title     string   `json:"title"`
	AvatarURL *string  `json:"avatar_url"`
	MemberIDs []string `json:"member_ids"`
}

// AddChatMembersRequest is the body for POST /chats/{chat_id}/members.
type AddChatMembersRequest struct {
	UserIDs []string `json:"user_ids"`
}

// LastMessagePreview is a small projection of the latest chat message.
type LastMessagePreview struct {
	ID        string    `json:"id"`
	SenderID  string    `json:"sender_id"`
	Type      string    `json:"type"`
	Text      *string   `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ChatSummary is one row in GET /chats.
type ChatSummary struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	Title       *string             `json:"title,omitempty"`
	AvatarURL   *string             `json:"avatar_url,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	LastMessage *LastMessagePreview `json:"last_message,omitempty"`
	UnreadCount int64               `json:"unread_count"`
}

// ChatListResponse is returned from GET /chats.
type ChatListResponse struct {
	Chats []ChatSummary `json:"chats"`
}

// ChatDetail is returned from GET /chats/{chat_id}.
type ChatDetail struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     *string   `json:"title,omitempty"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatMember is a member row without sensitive user fields.
type ChatMember struct {
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// ChatMembersResponse is returned from GET /chats/{chat_id}/members.
type ChatMembersResponse struct {
	Members []ChatMember `json:"members"`
}
