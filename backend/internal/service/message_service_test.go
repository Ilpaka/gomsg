package service

import (
	"context"
	"testing"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
)

func chatID() domain.ID {
	return domain.ID("30000000-0000-4000-8000-000000000003")
}

func newTestMessageService(msgs *fakeMsgRepo, chats *fakeChatRepo) (*MessageService, *[]repository.OutboxEnqueue) {
	ev := make([]repository.OutboxEnqueue, 0)
	ptr := &ev
	w := &fakeMessageWriter{msgs: msgs, chats: chats, events: ptr}
	return NewMessageService(msgs, chats, w, nil), ptr
}

func TestMessageService_Create_notMember(t *testing.T) {
	t.Parallel()
	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	svc, _ := newTestMessageService(msgs, chats)

	_, err := svc.Create(context.Background(), domain.ID("10000000-0000-4000-8000-000000000001"), string(chatID()), dto.CreateMessageRequest{
		Text: "hi",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestMessageService_Create_replyWrongChat(t *testing.T) {
	t.Parallel()
	cid := chatID()
	replyID := domain.ID("50000000-0000-4000-8000-000000000005")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == domain.ID("10000000-0000-4000-8000-000000000001") {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[replyID] = &domain.Message{
		ID:        replyID,
		ChatID:    domain.ID("99999999-9999-4999-8999-999999999999"),
		SenderID:  domain.ID("20000000-0000-4000-8000-000000000002"),
		Type:      domain.MessageTypeText,
		Text:      ptr("old"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, _ := newTestMessageService(msgs, chats)

	rid := string(replyID)
	_, err := svc.Create(context.Background(), domain.ID("10000000-0000-4000-8000-000000000001"), string(cid), dto.CreateMessageRequest{
		Text:      "reply",
		ReplyToID: &rid,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestMessageService_Create_success(t *testing.T) {
	t.Parallel()
	cid := chatID()
	actor := domain.ID("10000000-0000-4000-8000-000000000001")

	var lastChat domain.ID
	var lastMsg domain.ID
	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == actor {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
		updateLast: func(ctx context.Context, chatID, messageID domain.ID, at time.Time) error {
			lastChat = chatID
			lastMsg = messageID
			return nil
		},
	}
	msgs := newFakeMsgRepo()
	svc, evs := newTestMessageService(msgs, chats)

	out, err := svc.Create(context.Background(), actor, string(cid), dto.CreateMessageRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.ChatID != string(cid) || out.SenderID != string(actor) || out.Text == nil || *out.Text != "hello" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if lastChat != cid || lastMsg != domain.ID(out.ID) {
		t.Fatalf("UpdateLastMessage not called with expected ids: chat=%s msg=%s", lastChat, lastMsg)
	}
	if len(*evs) != 1 || (*evs)[0].EventType != domain.EventMessageCreated {
		t.Fatalf("expected outbox message.created, got %#v", *evs)
	}
}

func TestMessageService_Patch_notAuthor(t *testing.T) {
	t.Parallel()
	cid := chatID()
	msgID := domain.ID("50000000-0000-4000-8000-000000000005")
	author := domain.ID("20000000-0000-4000-8000-000000000002")
	other := domain.ID("10000000-0000-4000-8000-000000000001")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == other {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[msgID] = &domain.Message{
		ID:        msgID,
		ChatID:    cid,
		SenderID:  author,
		Type:      domain.MessageTypeText,
		Text:      ptr("x"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, _ := newTestMessageService(msgs, chats)

	_, err := svc.Patch(context.Background(), other, string(msgID), dto.PatchMessageRequest{Text: "nope"})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestMessageService_Patch_authorSuccess(t *testing.T) {
	t.Parallel()
	cid := chatID()
	msgID := domain.ID("50000000-0000-4000-8000-000000000005")
	author := domain.ID("10000000-0000-4000-8000-000000000001")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == author {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[msgID] = &domain.Message{
		ID:        msgID,
		ChatID:    cid,
		SenderID:  author,
		Type:      domain.MessageTypeText,
		Text:      ptr("old"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, evs := newTestMessageService(msgs, chats)

	out, err := svc.Patch(context.Background(), author, string(msgID), dto.PatchMessageRequest{Text: "new text"})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if out.Text == nil || *out.Text != "new text" {
		t.Fatalf("unexpected text: %+v", out.Text)
	}
	if len(*evs) != 1 || (*evs)[0].EventType != domain.EventMessageUpdated {
		t.Fatalf("expected outbox message.updated, got %#v", *evs)
	}
}

func TestMessageService_Delete_authorSuccess(t *testing.T) {
	t.Parallel()
	cid := chatID()
	msgID := domain.ID("50000000-0000-4000-8000-000000000005")
	author := domain.ID("10000000-0000-4000-8000-000000000001")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == author {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[msgID] = &domain.Message{
		ID:        msgID,
		ChatID:    cid,
		SenderID:  author,
		Type:      domain.MessageTypeText,
		Text:      ptr("x"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, evs := newTestMessageService(msgs, chats)

	if err := svc.Delete(context.Background(), author, string(msgID)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if msgs.byID[msgID].DeletedAt == nil {
		t.Fatal("expected soft delete")
	}
	if len(*evs) != 1 || (*evs)[0].EventType != domain.EventMessageDeleted {
		t.Fatalf("expected outbox message.deleted, got %#v", *evs)
	}
}

func TestMessageService_Delete_notAuthor(t *testing.T) {
	t.Parallel()
	cid := chatID()
	msgID := domain.ID("50000000-0000-4000-8000-000000000005")
	author := domain.ID("20000000-0000-4000-8000-000000000002")
	other := domain.ID("10000000-0000-4000-8000-000000000001")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == other {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[msgID] = &domain.Message{
		ID:        msgID,
		ChatID:    cid,
		SenderID:  author,
		Type:      domain.MessageTypeText,
		Text:      ptr("x"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, _ := newTestMessageService(msgs, chats)

	err := svc.Delete(context.Background(), other, string(msgID))
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func ptr(s string) *string { return &s }

func TestMessageService_MarkRead_successOutbox(t *testing.T) {
	t.Parallel()
	cid := chatID()
	msgID := domain.ID("50000000-0000-4000-8000-000000000005")
	reader := domain.ID("10000000-0000-4000-8000-000000000001")

	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == reader {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	msgs.byID[msgID] = &domain.Message{
		ID:        msgID,
		ChatID:    cid,
		SenderID:  domain.ID("20000000-0000-4000-8000-000000000002"),
		Type:      domain.MessageTypeText,
		Text:      ptr("x"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	svc, evs := newTestMessageService(msgs, chats)

	out, err := svc.MarkRead(context.Background(), reader, string(msgID))
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if out.ChatID != string(cid) || out.LastReadMessageID != string(msgID) {
		t.Fatalf("unexpected response: %+v", out)
	}
	if len(*evs) != 1 || (*evs)[0].EventType != domain.EventMessageReadReceipt {
		t.Fatalf("expected outbox message.read_receipt, got %#v", *evs)
	}
}

func TestValidateMessageTypeForMVP_rejectsImageAndFile(t *testing.T) {
	t.Parallel()
	if err := validateMessageTypeForMVP(domain.MessageTypeImage); err == nil {
		t.Fatal("expected error for image")
	}
	if err := validateMessageTypeForMVP(domain.MessageTypeFile); err == nil {
		t.Fatal("expected error for file")
	}
	if err := validateMessageTypeForMVP(domain.MessageTypeText); err != nil {
		t.Fatalf("text: %v", err)
	}
	if err := validateMessageTypeForMVP(domain.MessageTypeSystem); err != nil {
		t.Fatalf("system: %v", err)
	}
}
