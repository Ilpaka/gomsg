package service

import (
	"context"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// --- fake user repository ---

type fakeUserRepo struct {
	byEmail map[string]*domain.User
	byNick  map[string]*domain.User
	byID    map[domain.ID]*domain.User
	create  func(ctx context.Context, u *domain.User) error
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byEmail: make(map[string]*domain.User),
		byNick:  make(map[string]*domain.User),
		byID:    make(map[domain.ID]*domain.User),
	}
}

func (f *fakeUserRepo) Create(ctx context.Context, u *domain.User) error {
	if f.create != nil {
		return f.create(ctx, u)
	}
	if u.ID == "" {
		u.ID = domain.ID("70000000-0000-4000-8000-000000000007")
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	u.IsActive = true
	f.byEmail[u.Email] = u
	f.byNick[u.Nickname] = u
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id domain.ID) (*domain.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetByNickname(ctx context.Context, nickname string) (*domain.User, error) {
	u, ok := f.byNick[nickname]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) Search(ctx context.Context, query string, page repository.Page) ([]domain.User, error) {
	return nil, nil
}

func (f *fakeUserRepo) UpdateProfile(ctx context.Context, p repository.UpdateUserProfileParams) error {
	return nil
}

// --- fake session repository ---

type fakeSessionRepo struct {
	err    error
	byHash map[string]*domain.RefreshSession
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byHash: make(map[string]*domain.RefreshSession)}
}

func (f *fakeSessionRepo) Create(ctx context.Context, s *domain.RefreshSession) error {
	if f.err != nil {
		return f.err
	}
	if s.ID == "" {
		s.ID = domain.ID("80000000-0000-4000-8000-000000000008")
	}
	s.CreatedAt = time.Now().UTC()
	f.byHash[s.TokenHash] = s
	return nil
}

func (f *fakeSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error) {
	s, ok := f.byHash[tokenHash]
	if !ok || s.RevokedAt != nil {
		return nil, repository.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessionRepo) Revoke(ctx context.Context, id domain.ID) error { return nil }

func (f *fakeSessionRepo) RevokeAllByUser(ctx context.Context, userID domain.ID) error { return nil }

func (f *fakeSessionRepo) RotateRefresh(ctx context.Context, oldTokenHash string, newSess *domain.RefreshSession) error {
	s, ok := f.byHash[oldTokenHash]
	if !ok {
		return repository.ErrNotFound
	}
	delete(f.byHash, oldTokenHash)
	now := time.Now().UTC()
	s.RevokedAt = &now
	if newSess.ID == "" {
		newSess.ID = domain.ID("90000000-0000-4000-8000-000000000009")
	}
	newSess.CreatedAt = now
	f.byHash[newSess.TokenHash] = newSess
	return nil
}

// --- fake message writer (transactional path for tests) ---

type fakeMessageWriter struct {
	msgs   *fakeMsgRepo
	chats  *fakeChatRepo
	events *[]repository.OutboxEnqueue
}

func (f *fakeMessageWriter) CreateMessageWithOutbox(ctx context.Context, m *domain.Message, ob repository.OutboxEnqueue) error {
	if err := f.msgs.Create(ctx, m); err != nil {
		return err
	}
	if f.chats != nil {
		_ = f.chats.UpdateLastMessage(ctx, m.ChatID, m.ID, m.CreatedAt)
	}
	if f.events != nil {
		*f.events = append(*f.events, ob)
	}
	return nil
}

func (f *fakeMessageWriter) UpdateMessageTextWithOutbox(ctx context.Context, chatID, messageID domain.ID, text string, ob repository.OutboxEnqueue) error {
	if err := f.msgs.UpdateText(ctx, chatID, messageID, text); err != nil {
		return err
	}
	if f.events != nil {
		*f.events = append(*f.events, ob)
	}
	return nil
}

func (f *fakeMessageWriter) SoftDeleteMessageWithOutbox(ctx context.Context, chatID, messageID domain.ID, ob repository.OutboxEnqueue) error {
	if err := f.msgs.SoftDelete(ctx, chatID, messageID); err != nil {
		return err
	}
	if f.events != nil {
		*f.events = append(*f.events, ob)
	}
	return nil
}

func (f *fakeMessageWriter) UpdateMemberReadWithOutbox(ctx context.Context, chatID, userID, readUpToMessageID domain.ID, ob repository.OutboxEnqueue) error {
	if f.chats != nil {
		_ = f.chats.UpdateMemberRead(ctx, chatID, userID, readUpToMessageID)
	}
	if f.events != nil {
		*f.events = append(*f.events, ob)
	}
	return nil
}

// --- fake chat repository ---

type fakeChatRepo struct {
	getDirectByKey func(ctx context.Context, key string) (*domain.Chat, error)
	createChat     func(ctx context.Context, c *domain.Chat) error
	addMembers     func(ctx context.Context, chatID domain.ID, members []repository.AddChatMember) error
	findMember     func(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error)
	createGroup    func(ctx context.Context, title string, avatar *string, creator domain.ID, others []domain.ID) (*domain.Chat, error)
	updateLast     func(ctx context.Context, chatID, messageID domain.ID, at time.Time) error
}

func (f *fakeChatRepo) CreateChat(ctx context.Context, c *domain.Chat) error {
	if f.createChat != nil {
		return f.createChat(ctx, c)
	}
	if c.ID == "" {
		c.ID = domain.ID("30000000-0000-4000-8000-000000000003")
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (f *fakeChatRepo) CreateMember(ctx context.Context, m *domain.ChatMember) error { return nil }

func (f *fakeChatRepo) GetByID(ctx context.Context, id domain.ID) (*domain.Chat, error) {
	return nil, repository.ErrNotFound
}

func (f *fakeChatRepo) GetDirectByKey(ctx context.Context, directKey string) (*domain.Chat, error) {
	if f.getDirectByKey != nil {
		return f.getDirectByKey(ctx, directKey)
	}
	return nil, repository.ErrNotFound
}

func (f *fakeChatRepo) GetUserChats(ctx context.Context, userID domain.ID, page repository.Page) ([]domain.Chat, error) {
	return nil, nil
}

func (f *fakeChatRepo) ListUserChatsSummary(ctx context.Context, userID domain.ID, page repository.Page) ([]repository.ChatListRow, error) {
	return nil, nil
}

func (f *fakeChatRepo) GetChatMembers(ctx context.Context, chatID domain.ID) ([]domain.ChatMember, error) {
	return nil, nil
}

func (f *fakeChatRepo) FindMembership(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error) {
	if f.findMember != nil {
		return f.findMember(ctx, chatID, userID)
	}
	return nil, repository.ErrNotFound
}

func (f *fakeChatRepo) AddMembers(ctx context.Context, chatID domain.ID, members []repository.AddChatMember) error {
	if f.addMembers != nil {
		return f.addMembers(ctx, chatID, members)
	}
	return nil
}

func (f *fakeChatRepo) RemoveMember(ctx context.Context, chatID, userID domain.ID) error { return nil }

func (f *fakeChatRepo) CreateGroupWithMembers(ctx context.Context, title string, avatarURL *string, creator domain.ID, otherUserIDs []domain.ID) (*domain.Chat, error) {
	if f.createGroup != nil {
		return f.createGroup(ctx, title, avatarURL, creator, otherUserIDs)
	}
	ch := &domain.Chat{
		ID:        domain.ID("40000000-0000-4000-8000-000000000004"),
		Type:      domain.ChatTypeGroup,
		Title:     &title,
		CreatedBy: &creator,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	return ch, nil
}

func (f *fakeChatRepo) UpdateLastMessage(ctx context.Context, chatID, messageID domain.ID, at time.Time) error {
	if f.updateLast != nil {
		return f.updateLast(ctx, chatID, messageID, at)
	}
	return nil
}

func (f *fakeChatRepo) UpdateMemberRead(ctx context.Context, chatID, userID, readUpToMessageID domain.ID) error {
	return nil
}

// --- fake message repository ---

type fakeMsgRepo struct {
	byID   map[domain.ID]*domain.Message
	create func(ctx context.Context, m *domain.Message) error
	update func(ctx context.Context, chatID, messageID domain.ID, text string) error
	delete func(ctx context.Context, chatID, messageID domain.ID) error
}

func newFakeMsgRepo() *fakeMsgRepo {
	return &fakeMsgRepo{byID: make(map[domain.ID]*domain.Message)}
}

func (f *fakeMsgRepo) Create(ctx context.Context, m *domain.Message) error {
	if f.create != nil {
		return f.create(ctx, m)
	}
	if m.ID == "" {
		m.ID = domain.ID("50000000-0000-4000-8000-000000000005")
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	f.byID[m.ID] = m
	return nil
}

func (f *fakeMsgRepo) GetByID(ctx context.Context, id domain.ID) (*domain.Message, error) {
	m, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return m, nil
}

func (f *fakeMsgRepo) GetChatMessages(ctx context.Context, chatID domain.ID, opts repository.MessageListOpts) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeMsgRepo) UpdateText(ctx context.Context, chatID, messageID domain.ID, text string) error {
	if f.update != nil {
		return f.update(ctx, chatID, messageID, text)
	}
	m, ok := f.byID[messageID]
	if !ok {
		return repository.ErrNotFound
	}
	m.Text = &text
	m.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeMsgRepo) SoftDelete(ctx context.Context, chatID, messageID domain.ID) error {
	if f.delete != nil {
		return f.delete(ctx, chatID, messageID)
	}
	m, ok := f.byID[messageID]
	if !ok {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	m.DeletedAt = &now
	return nil
}

func (f *fakeMsgRepo) CountUnreadAfter(ctx context.Context, chatID domain.ID, afterMessageID *domain.ID) (int64, error) {
	return 0, nil
}
