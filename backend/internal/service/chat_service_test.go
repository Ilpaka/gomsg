package service

import (
	"context"
	"testing"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
)

func TestChatService_CreateDirect_returnsExistingWithoutDuplicate(t *testing.T) {
	t.Parallel()
	actor := domain.ID("10000000-0000-4000-8000-000000000001")
	peer := "20000000-0000-4000-8000-000000000002"

	users := newFakeUserRepo()
	users.byID[domain.ID(peer)] = &domain.User{ID: domain.ID(peer), Email: "p@example.com", Nickname: "peer_m", IsActive: true}

	var stored *domain.Chat
	chats := &fakeChatRepo{
		getDirectByKey: func(ctx context.Context, key string) (*domain.Chat, error) {
			if stored != nil && stored.DirectKey != nil && *stored.DirectKey == key {
				return stored, nil
			}
			return nil, repository.ErrNotFound
		},
		createChat: func(ctx context.Context, c *domain.Chat) error {
			c.ID = domain.ID("30000000-0000-4000-8000-000000000003")
			dk := *c.DirectKey
			cc := *c
			cc.DirectKey = &dk
			stored = &cc
			return nil
		},
		findMember: func(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error) {
			if stored == nil || chatID != stored.ID {
				return nil, repository.ErrNotFound
			}
			if userID == actor || userID == domain.ID(peer) {
				return &domain.ChatMember{ChatID: chatID, UserID: userID}, nil
			}
			return nil, repository.ErrNotFound
		},
	}

	svc := NewChatService(chats, users)

	first, err := svc.CreateDirect(context.Background(), actor, dto.CreateDirectChatRequest{UserID: peer})
	if err != nil {
		t.Fatalf("first CreateDirect: %v", err)
	}
	second, err := svc.CreateDirect(context.Background(), actor, dto.CreateDirectChatRequest{UserID: peer})
	if err != nil {
		t.Fatalf("second CreateDirect: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same chat id, got %s vs %s", first.ID, second.ID)
	}
}

func TestChatService_CreateGroup_validation(t *testing.T) {
	t.Parallel()
	svc := NewChatService(&fakeChatRepo{}, newFakeUserRepo())
	_, err := svc.CreateGroup(context.Background(), domain.ID("10000000-0000-4000-8000-000000000001"), dto.CreateGroupChatRequest{
		Title: " ",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestChatService_CreateGroup_success(t *testing.T) {
	t.Parallel()
	actor := domain.ID("10000000-0000-4000-8000-000000000001")
	m1 := domain.ID("20000000-0000-4000-8000-000000000002")

	users := newFakeUserRepo()
	users.byID[m1] = &domain.User{ID: m1, Email: "m1@example.com", Nickname: "m1_m", IsActive: true}

	chats := &fakeChatRepo{}
	svc := NewChatService(chats, users)

	out, err := svc.CreateGroup(context.Background(), actor, dto.CreateGroupChatRequest{
		Title:     "Team",
		MemberIDs: []string{string(m1)},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if out.Type != string(domain.ChatTypeGroup) || out.Title == nil || *out.Title != "Team" {
		t.Fatalf("unexpected chat: %+v", out)
	}
}
