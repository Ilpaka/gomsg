package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	"goflow/backend/internal/observability/metrics"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"

	"github.com/google/uuid"
)

var msgUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	msgTextMaxLen     = 8000
	msgDefaultLimit   = 50
	msgMaxLimit       = 200
	msgBeforeTimeForm = time.RFC3339Nano
)

// MessageService handles message CRUD, listing, and read receipts.
// writer performs transactional message persistence + outbox enqueue (same path for REST and WS).
type MessageService struct {
	msgs    repository.MessageRepository
	chats   repository.ChatRepository
	writer  repository.MessageWriter
	metrics *metrics.M
}

func NewMessageService(msgs repository.MessageRepository, chats repository.ChatRepository, writer repository.MessageWriter, m *metrics.M) *MessageService {
	return &MessageService{msgs: msgs, chats: chats, writer: writer, metrics: m}
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
	if s.writer == nil {
		return nil, apperr.Internal("message writer not configured", nil)
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
	if err := validateMessageTypeForMVP(m.Type); err != nil {
		return nil, err
	}

	ob, err := s.outboxForMessageCreated(m)
	if err != nil {
		return nil, err
	}
	if err := s.writer.CreateMessageWithOutbox(ctx, m, ob); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("chat not found")
		}
		return nil, apperr.Internal("create message", err)
	}
	if s.metrics != nil {
		s.metrics.MessagesCreate.Inc()
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
	if s.writer == nil {
		return nil, apperr.Internal("message writer not configured", nil)
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
	if err := validateMessageTypeForMVP(m.Type); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, apperr.Validation("text is required", nil)
	}
	if len(text) > msgTextMaxLen {
		return nil, apperr.Validation("text too long", nil)
	}

	ob, err := s.outboxForMessageUpdated(m, text)
	if err != nil {
		return nil, err
	}
	if err := s.writer.UpdateMessageTextWithOutbox(ctx, m.ChatID, m.ID, text, ob); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("message not found")
		}
		return nil, apperr.Internal("update message", err)
	}
	m2, err := s.msgs.GetByID(ctx, m.ID)
	if err != nil {
		return nil, apperr.Internal("reload message", err)
	}
	if s.metrics != nil {
		s.metrics.MessagesUpdate.Inc()
	}
	out := toMessageDTO(m2)
	return &out, nil
}

func (s *MessageService) Delete(ctx context.Context, actor domain.ID, messageIDRaw string) error {
	if actor == "" {
		return apperr.Unauthorized("missing user")
	}
	if s.writer == nil {
		return apperr.Internal("message writer not configured", nil)
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

	ob, err := s.outboxForMessageDeleted(m)
	if err != nil {
		return err
	}
	if err := s.writer.SoftDeleteMessageWithOutbox(ctx, m.ChatID, m.ID, ob); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("message not found")
		}
		return apperr.Internal("delete message", err)
	}
	if s.metrics != nil {
		s.metrics.MessagesDelete.Inc()
	}
	return nil
}

func (s *MessageService) MarkRead(ctx context.Context, actor domain.ID, messageIDRaw string) (*dto.MarkReadResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	if s.writer == nil {
		return nil, apperr.Internal("message writer not configured", nil)
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

	ob, err := s.outboxForReadReceipt(m.ChatID, actor, m.ID)
	if err != nil {
		return nil, err
	}
	if err := s.writer.UpdateMemberReadWithOutbox(ctx, m.ChatID, actor, m.ID, ob); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("membership not found")
		}
		return nil, apperr.Internal("update read state", err)
	}
	if s.metrics != nil {
		s.metrics.MessagesReadRc.Inc()
	}
	return &dto.MarkReadResponse{
		ChatID:            string(m.ChatID),
		LastReadMessageID: string(m.ID),
	}, nil
}

func validateMessageTypeForMVP(t domain.MessageType) error {
	switch t {
	case domain.MessageTypeText, domain.MessageTypeSystem:
		return nil
	default:
		return apperr.Validation("unsupported message type for MVP (only text and system)", nil)
	}
}

func newOutboxEventID() domain.ID {
	return domain.ID(uuid.NewString())
}

func (s *MessageService) outboxForMessageCreated(m *domain.Message) (repository.OutboxEnqueue, error) {
	cid := m.ChatID
	return repository.OutboxEnqueue{
		EventID:       newOutboxEventID(),
		EventType:     domain.EventMessageCreated,
		AggregateType: domain.AggregateTypeMessage,
		AggregateID:   "", // filled by writer after insert
		ChatID:        &cid,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       nil, // writer builds JSON after message row exists
	}, nil
}

func (s *MessageService) outboxForMessageUpdated(m *domain.Message, _ string) (repository.OutboxEnqueue, error) {
	cid := m.ChatID
	return repository.OutboxEnqueue{
		EventID:       newOutboxEventID(),
		EventType:     domain.EventMessageUpdated,
		AggregateType: domain.AggregateTypeMessage,
		AggregateID:   string(m.ID),
		ChatID:        &cid,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       nil, // writer builds from DB row after UPDATE
	}, nil
}

func (s *MessageService) outboxForMessageDeleted(m *domain.Message) (repository.OutboxEnqueue, error) {
	now := time.Now().UTC()
	body := map[string]any{
		"id":         string(m.ID),
		"chat_id":    string(m.ChatID),
		"deleted_at": now.Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return repository.OutboxEnqueue{}, err
	}
	cid := m.ChatID
	return repository.OutboxEnqueue{
		EventID:       newOutboxEventID(),
		EventType:     domain.EventMessageDeleted,
		AggregateType: domain.AggregateTypeMessage,
		AggregateID:   string(m.ID),
		ChatID:        &cid,
		OccurredAt:    now,
		Version:       1,
		Payload:       raw,
	}, nil
}

func (s *MessageService) outboxForReadReceipt(chatID, userID, messageID domain.ID) (repository.OutboxEnqueue, error) {
	body := map[string]any{
		"chat_id":               string(chatID),
		"user_id":               string(userID),
		"last_read_message_id":  string(messageID),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return repository.OutboxEnqueue{}, err
	}
	cid := chatID
	return repository.OutboxEnqueue{
		EventID:       newOutboxEventID(),
		EventType:     domain.EventMessageReadReceipt,
		AggregateType: domain.AggregateTypeReadState,
		AggregateID:   string(userID) + ":" + string(messageID),
		ChatID:        &cid,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       raw,
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
