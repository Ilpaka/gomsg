package service

import (
	"context"
	"testing"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	"goflow/backend/internal/observability/metrics"
	"goflow/backend/internal/repository"
)

func TestMessageService_Create_incrementsMessageCounter(t *testing.T) {
	t.Parallel()
	cid := chatID()
	actor := domain.ID("10000000-0000-4000-8000-000000000001")
	chats := &fakeChatRepo{
		findMember: func(ctx context.Context, cID, userID domain.ID) (*domain.ChatMember, error) {
			if cID == cid && userID == actor {
				return &domain.ChatMember{}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	msgs := newFakeMsgRepo()
	met := metrics.New()
	w := &fakeMessageWriter{msgs: msgs, chats: chats, events: new([]repository.OutboxEnqueue)}
	svc := NewMessageService(msgs, chats, w, met)

	_, err := svc.Create(context.Background(), actor, string(cid), dto.CreateMessageRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	gather, err := met.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	var found float64
	for _, mf := range gather {
		if mf.GetName() != "messages_created_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			found += m.GetCounter().GetValue()
		}
	}
	if found < 1 {
		t.Fatalf("expected messages_created_total >= 1, got %v", found)
	}
}
