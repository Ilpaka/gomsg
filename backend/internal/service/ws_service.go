package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
	wstransport "goflow/backend/internal/transport/ws"
)

var _ wstransport.EventProcessor = (*WSService)(nil)

// WSService wires websocket inbound events to MessageService and chat fan-out.
type WSService struct {
	msgs  *MessageService
	chats repository.ChatRepository
	bc    *wstransport.Broadcaster
}

func NewWSService(msgs *MessageService, chats repository.ChatRepository, bc *wstransport.Broadcaster) *WSService {
	return &WSService{msgs: msgs, chats: chats, bc: bc}
}

// HandleEvent implements wstransport.EventProcessor.
func (s *WSService) HandleEvent(ctx context.Context, userID domain.ID, payload []byte) ([][]byte, error) {
	var env struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
		Meta  json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
			"code":    "validation_failed",
			"message": "invalid json envelope",
		}, metaOrEmpty(env.Meta))
		return [][]byte{b}, nil
	}
	meta := metaOrEmpty(env.Meta)

	switch env.Event {
	case wstransport.EventPing:
		b, err := wstransport.MarshalEnvelope(wstransport.EventPong, map[string]any{}, meta)
		if err != nil {
			return nil, err
		}
		return [][]byte{b}, nil

	case wstransport.EventMessageSend:
		return s.handleMessageSend(ctx, userID, env.Data, meta)

	case wstransport.EventMessageRead:
		return s.handleMessageRead(ctx, userID, env.Data, meta)

	case wstransport.EventTypingStart, wstransport.EventTypingStop:
		return s.handleTyping(ctx, userID, env.Event, env.Data, meta)

	default:
		b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
			"code":    "validation_failed",
			"message": "unknown event",
		}, meta)
		return [][]byte{b}, nil
	}
}

func (s *WSService) handleMessageSend(ctx context.Context, userID domain.ID, data json.RawMessage, meta json.RawMessage) ([][]byte, error) {
	var in struct {
		ChatID    string  `json:"chat_id"`
		Text      string  `json:"text"`
		ReplyToID *string `json:"reply_to_id"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
			"code":    "validation_failed",
			"message": "invalid message.send data",
		}, metaOrEmpty(meta))
		return [][]byte{b}, nil
	}
	msg, err := s.msgs.Create(ctx, userID, strings.TrimSpace(in.ChatID), dto.CreateMessageRequest{
		Text:      in.Text,
		ReplyToID: in.ReplyToID,
	})
	if err != nil {
		return [][]byte{wsErrorFrame(err, meta)}, nil
	}
	_ = s.bc.PublishToChat(ctx, domain.ID(msg.ChatID), wstransport.EventMessageCreated, msg, metaOrEmpty(meta))
	return nil, nil
}

func (s *WSService) handleMessageRead(ctx context.Context, userID domain.ID, data json.RawMessage, meta json.RawMessage) ([][]byte, error) {
	var in struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
			"code":    "validation_failed",
			"message": "invalid message.read data",
		}, metaOrEmpty(meta))
		return [][]byte{b}, nil
	}
	out, err := s.msgs.MarkRead(ctx, userID, strings.TrimSpace(in.MessageID))
	if err != nil {
		return [][]byte{wsErrorFrame(err, meta)}, nil
	}
	readPayload := map[string]any{
		"chat_id":              out.ChatID,
		"user_id":              string(userID),
		"last_read_message_id": out.LastReadMessageID,
	}
	_ = s.bc.PublishToChat(ctx, domain.ID(out.ChatID), wstransport.EventMessageRead, readPayload, metaOrEmpty(meta))
	return nil, nil
}

func (s *WSService) handleTyping(ctx context.Context, userID domain.ID, event string, data json.RawMessage, meta json.RawMessage) ([][]byte, error) {
	var in struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
			"code":    "validation_failed",
			"message": "invalid typing payload",
		}, metaOrEmpty(meta))
		return [][]byte{b}, nil
	}
	cid, err := parseMsgChatID(strings.TrimSpace(in.ChatID))
	if err != nil {
		return [][]byte{wsErrorFrame(err, meta)}, nil
	}
	if _, err := s.chats.FindMembership(ctx, cid, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			b, _ := wstransport.MarshalEnvelope(wstransport.EventError, map[string]any{
				"code":    "forbidden",
				"message": "not a chat member",
			}, metaOrEmpty(meta))
			return [][]byte{b}, nil
		}
		return [][]byte{wsErrorFrame(err, meta)}, nil
	}
	outEvent := wstransport.EventTypingStarted
	if event == wstransport.EventTypingStop {
		outEvent = wstransport.EventTypingStopped
	}
	payload := map[string]any{
		"chat_id": string(cid),
		"user_id": string(userID),
	}
	_ = s.bc.PublishToChat(ctx, cid, outEvent, payload, metaOrEmpty(meta))
	return nil, nil
}

func wsErrorFrame(err error, meta json.RawMessage) []byte {
	body := map[string]any{"code": "internal", "message": "error"}
	if ae, ok := apperr.As(err); ok {
		body["code"] = string(ae.Kind)
		body["message"] = ae.Message
	} else if err != nil {
		body["message"] = err.Error()
	}
	b, _ := wstransport.MarshalEnvelope(wstransport.EventError, body, metaOrEmpty(meta))
	return b
}

func metaOrEmpty(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	return m
}
