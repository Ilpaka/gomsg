package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
)

var msgUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	msgTextMaxLen     = 8000
	msgDefaultLimit   = 50
	msgMaxLimit       = 200
	msgBeforeTimeForm = time.RFC3339Nano
)

// MessageService handles message CRUD, listing, and read receipts.
type MessageService struct {
	msgs  repository.MessageRepository
	chats repository.ChatRepository
}

func NewMessageService(msgs repository.MessageRepository, chats repository.ChatRepository) *MessageService {
	return &MessageService{msgs: msgs, chats: chats}
}

func (s *MessageService) ListByChat(ctx context.Context, actor domain.ID, chatIDRaw string, limit int, beforeRaw string) (*dto.MessageListResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	cid, err := parseMsgChatID(chatIDRaw)
	if err != nil {
		return nil, err
	}
	if err := s.requireChatMember(ctx, cid, actor); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = msgDefaultLimit
	}
	if limit > msgMaxLimit {
		limit = msgMaxLimit
	}
	beforeID, beforeTime, err := parseBeforeCursor(beforeRaw)
	if err != nil {
		return nil, err
	}
	rows, err := s.msgs.GetChatMessages(ctx, cid, repository.MessageListOpts{
		Limit:      limit,
		BeforeID:   beforeID,
		BeforeTime: beforeTime,
	})
	if err != nil {
		return nil, apperr.Internal("list messages", err)
	}
	out := make([]dto.Message, 0, len(rows))
	for i := range rows {
		out = append(out, toMessageDTO(&rows[i]))
	}
	return &dto.MessageListResponse{
		Messages: out,
		HasMore:  len(out) == limit,
	}, nil
}

func (s *MessageService) Create(ctx context.Context, actor domain.ID, chatIDRaw string, in dto.CreateMessageRequest) (*dto.Message, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	cid, err := parseMsgChatID(chatIDRaw)
	if err != nil {
		return nil, err
	}
	if err := s.requireChatMember(ctx, cid, actor); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, apperr.Validation("text is required", nil)
	}
	if len(text) > msgTextMaxLen {
		return nil, apperr.Validation("text too long", nil)
	}
	var reply *domain.ID
	if in.ReplyToID != nil && strings.TrimSpace(*in.ReplyToID) != "" {
		rid := strings.TrimSpace(*in.ReplyToID)
		if !msgUUIDRe.MatchString(rid) {
			return nil, apperr.Validation("invalid reply_to_id", nil)
		}
		rm, err := s.msgs.GetByID(ctx, domain.ID(rid))
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, apperr.NotFound("reply message not found")
			}
			return nil, apperr.Internal("load reply message", err)
		}
		if rm.ChatID != cid {
			return nil, apperr.Validation("reply_to_id must belong to the same chat", nil)
		}
		if rm.DeletedAt != nil {
			return nil, apperr.Validation("cannot reply to a deleted message", nil)
		}
		id := domain.ID(rid)
		reply = &id
	}

	m := &domain.Message{
		ChatID:    cid,
		SenderID:  actor,
		Type:      domain.MessageTypeText,
		Text:      &text,
		ReplyToID: reply,
	}
	if err := s.msgs.Create(ctx, m); err != nil {
		return nil, apperr.Internal("create message", err)
	}
	if err := s.chats.UpdateLastMessage(ctx, cid, m.ID, m.CreatedAt); err != nil {
		return nil, apperr.Internal("update chat last message", err)
	}
	out := toMessageDTO(m)
	return &out, nil
}

func (s *MessageService) Get(ctx context.Context, actor domain.ID, messageIDRaw string) (*dto.Message, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	mid, err := parseMsgID(messageIDRaw)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.GetByID(ctx, mid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("message not found")
		}
		return nil, apperr.Internal("load message", err)
	}
	if m.DeletedAt != nil {
		return nil, apperr.NotFound("message not found")
	}
	if err := s.requireChatMember(ctx, m.ChatID, actor); err != nil {
		return nil, err
	}
	out := toMessageDTO(m)
	return &out, nil
}

func (s *MessageService) Patch(ctx context.Context, actor domain.ID, messageIDRaw string, in dto.PatchMessageRequest) (*dto.Message, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	mid, err := parseMsgID(messageIDRaw)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.GetByID(ctx, mid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("message not found")
		}
		return nil, apperr.Internal("load message", err)
	}
	if m.DeletedAt != nil {
		return nil, apperr.NotFound("message not found")
	}
	if m.SenderID != actor {
		return nil, apperr.Forbidden("only author can edit message")
	}
	if err := s.requireChatMember(ctx, m.ChatID, actor); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, apperr.Validation("text is required", nil)
	}
	if len(text) > msgTextMaxLen {
		return nil, apperr.Validation("text too long", nil)
	}
	if err := s.msgs.UpdateText(ctx, m.ChatID, m.ID, text); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("message not found")
		}
		return nil, apperr.Internal("update message", err)
	}
	m2, err := s.msgs.GetByID(ctx, m.ID)
	if err != nil {
		return nil, apperr.Internal("reload message", err)
	}
	out := toMessageDTO(m2)
	return &out, nil
}

func (s *MessageService) Delete(ctx context.Context, actor domain.ID, messageIDRaw string) error {
	if actor == "" {
		return apperr.Unauthorized("missing user")
	}
	mid, err := parseMsgID(messageIDRaw)
	if err != nil {
		return err
	}
	m, err := s.msgs.GetByID(ctx, mid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("message not found")
		}
		return apperr.Internal("load message", err)
	}
	if m.DeletedAt != nil {
		return apperr.NotFound("message not found")
	}
	if m.SenderID != actor {
		return apperr.Forbidden("only author can delete message")
	}
	if err := s.requireChatMember(ctx, m.ChatID, actor); err != nil {
		return err
	}
	if err := s.msgs.SoftDelete(ctx, m.ChatID, m.ID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("message not found")
		}
		return apperr.Internal("delete message", err)
	}
	return nil
}

func (s *MessageService) MarkRead(ctx context.Context, actor domain.ID, messageIDRaw string) (*dto.MarkReadResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	mid, err := parseMsgID(messageIDRaw)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.GetByID(ctx, mid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("message not found")
		}
		return nil, apperr.Internal("load message", err)
	}
	if m.DeletedAt != nil {
		return nil, apperr.NotFound("message not found")
	}
	if err := s.requireChatMember(ctx, m.ChatID, actor); err != nil {
		return nil, err
	}
	if err := s.chats.UpdateMemberRead(ctx, m.ChatID, actor, m.ID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("membership not found")
		}
		return nil, apperr.Internal("update read state", err)
	}
	return &dto.MarkReadResponse{
		ChatID:            string(m.ChatID),
		LastReadMessageID: string(m.ID),
	}, nil
}

func (s *MessageService) requireChatMember(ctx context.Context, chatID, userID domain.ID) error {
	_, err := s.chats.FindMembership(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.Forbidden("not a chat member")
		}
		return apperr.Internal("lookup membership", err)
	}
	return nil
}

func toMessageDTO(m *domain.Message) dto.Message {
	if m == nil {
		return dto.Message{}
	}
	var reply *string
	if m.ReplyToID != nil {
		s := string(*m.ReplyToID)
		reply = &s
	}
	return dto.Message{
		ID:        string(m.ID),
		ChatID:    string(m.ChatID),
		SenderID:  string(m.SenderID),
		Type:      string(m.Type),
		Text:      m.Text,
		ReplyToID: reply,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func parseMsgChatID(raw string) (domain.ID, error) {
	raw = strings.TrimSpace(raw)
	if !msgUUIDRe.MatchString(raw) {
		return "", apperr.Validation("invalid chat_id", nil)
	}
	return domain.ID(raw), nil
}

func parseMsgID(raw string) (domain.ID, error) {
	raw = strings.TrimSpace(raw)
	if !msgUUIDRe.MatchString(raw) {
		return "", apperr.Validation("invalid message_id", nil)
	}
	return domain.ID(raw), nil
}

func parseBeforeCursor(raw string) (*domain.ID, *time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}
	if msgUUIDRe.MatchString(raw) {
		id := domain.ID(raw)
		return &id, nil, nil
	}
	if t, err := time.Parse(msgBeforeTimeForm, raw); err == nil {
		return nil, &t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return nil, &t, nil
	}
	return nil, nil, apperr.Validation("invalid before cursor (use message id or RFC3339/RFC3339Nano timestamp)", nil)
}
