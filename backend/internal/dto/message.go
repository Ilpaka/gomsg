package dto

import "time"

// CreateMessageRequest is the body for POST /chats/{chat_id}/messages (MVP: text only).
type CreateMessageRequest struct {
	Text      string  `json:"text"`
	ReplyToID *string `json:"reply_to_id"`
}

// PatchMessageRequest is the body for PATCH /messages/{message_id}.
type PatchMessageRequest struct {
	Text string `json:"text"`
}

// Message is a safe API projection of a chat message.
type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	Type      string    `json:"type"`
	Text      *string   `json:"text,omitempty"`
	ReplyToID *string   `json:"reply_to_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageListResponse is returned from GET /chats/{chat_id}/messages.
// Messages are ordered newest-first (same as scrolling up in typical messengers).
type MessageListResponse struct {
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"has_more"`
}

// MarkReadResponse is returned from POST /messages/{message_id}/read.
type MarkReadResponse struct {
	ChatID            string `json:"chat_id"`
	LastReadMessageID string `json:"last_read_message_id"`
}
