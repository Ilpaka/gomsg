package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
)

var chatUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	chatListDefaultLimit = 30
	chatListMaxLimit     = 100
	chatTitleMaxLen      = 255
	chatAvatarMaxLen     = 2048
)

// ChatService implements chat creation, listing, membership, and authorization rules.
type ChatService struct {
	chats repository.ChatRepository
	users repository.UserRepository
}

func NewChatService(chats repository.ChatRepository, users repository.UserRepository) *ChatService {
	return &ChatService{chats: chats, users: users}
}

// CreateDirect opens an existing direct chat by direct_key or creates one with both members (roles: member).
func (s *ChatService) CreateDirect(ctx context.Context, actor domain.ID, in dto.CreateDirectChatRequest) (*dto.ChatDetail, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	peer := strings.TrimSpace(in.UserID)
	if peer == "" {
		return nil, apperr.Validation("user_id is required", nil)
	}
	if !chatUUIDRe.MatchString(peer) {
		return nil, apperr.Validation("invalid user_id", nil)
	}
	if domain.ID(peer) == actor {
		return nil, apperr.Validation("cannot start direct chat with yourself", nil)
	}
	if _, err := s.users.GetByID(ctx, domain.ID(peer)); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("load peer user", err)
	}

	key, err := directKey(actor, domain.ID(peer))
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		existing, err := s.chats.GetDirectByKey(ctx, key)
		if err == nil {
			if err := s.requireMember(ctx, existing.ID, actor); err != nil {
				return nil, err
			}
			_ = s.chats.AddMembers(ctx, existing.ID, []repository.AddChatMember{
				{UserID: actor, Role: domain.ChatMemberRoleMember},
				{UserID: domain.ID(peer), Role: domain.ChatMemberRoleMember},
			})
			return s.chatDetail(ctx, existing)
		}
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.Internal("lookup direct chat", err)
		}

		dk := key
		chat := &domain.Chat{
			Type:      domain.ChatTypeDirect,
			DirectKey: &dk,
		}
		if err := s.chats.CreateChat(ctx, chat); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return nil, apperr.Internal("create direct chat", err)
		}
		if err := s.chats.AddMembers(ctx, chat.ID, []repository.AddChatMember{
			{UserID: actor, Role: domain.ChatMemberRoleMember},
			{UserID: domain.ID(peer), Role: domain.ChatMemberRoleMember},
		}); err != nil {
			return nil, apperr.Internal("add direct members", err)
		}
		return s.chatDetail(ctx, chat)
	}
	return nil, apperr.Conflict("could not create or resolve direct chat")
}

// CreateGroup creates a group chat; creator is owner; member_ids become members (transactional in repository).
func (s *ChatService) CreateGroup(ctx context.Context, actor domain.ID, in dto.CreateGroupChatRequest) (*dto.ChatDetail, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, apperr.Validation("title is required", nil)
	}
	if len(title) > chatTitleMaxLen {
		return nil, apperr.Validation("title too long", nil)
	}
	var avatar *string
	if in.AvatarURL != nil {
		v := strings.TrimSpace(*in.AvatarURL)
		if len(v) > chatAvatarMaxLen {
			return nil, apperr.Validation("avatar_url too long", nil)
		}
		if v != "" {
			avatar = &v
		}
	}

	others := make([]domain.ID, 0, len(in.MemberIDs))
	seen := map[domain.ID]struct{}{actor: {}}
	for _, raw := range in.MemberIDs {
		id := strings.TrimSpace(raw)
		if id == "" || !chatUUIDRe.MatchString(id) {
			return nil, apperr.Validation("invalid member_ids entry", nil)
		}
		uid := domain.ID(id)
		if uid == actor {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if _, err := s.users.GetByID(ctx, uid); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, apperr.NotFound("member user not found")
			}
			return nil, apperr.Internal("load member user", err)
		}
		others = append(others, uid)
	}

	c, err := s.chats.CreateGroupWithMembers(ctx, title, avatar, actor, others)
	if err != nil {
		return nil, apperr.Internal("create group chat", err)
	}
	return s.chatDetail(ctx, c)
}

// ListMine returns chats for the current user with last message preview and unread_count (SQL in repository).
func (s *ChatService) ListMine(ctx context.Context, actor domain.ID, limit, offset int) (*dto.ChatListResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	if limit <= 0 {
		limit = chatListDefaultLimit
	}
	if limit > chatListMaxLimit {
		limit = chatListMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.chats.ListUserChatsSummary(ctx, actor, repository.Page{Limit: limit, Offset: offset})
	if err != nil {
		return nil, apperr.Internal("list chats", err)
	}
	out := make([]dto.ChatSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, toChatSummary(row))
	}
	return &dto.ChatListResponse{Chats: out}, nil
}

// Get returns chat details if the user is a member.
func (s *ChatService) Get(ctx context.Context, actor domain.ID, chatID string) (*dto.ChatDetail, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	cid, err := parseChatID(chatID)
	if err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, cid, actor); err != nil {
		return nil, err
	}
	c, err := s.chats.GetByID(ctx, cid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("chat not found")
		}
		return nil, apperr.Internal("load chat", err)
	}
	if c.IsDeleted {
		return nil, apperr.NotFound("chat not found")
	}
	return s.chatDetail(ctx, c)
}

// Members returns chat members if the user belongs to the chat.
func (s *ChatService) Members(ctx context.Context, actor domain.ID, chatID string) (*dto.ChatMembersResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	cid, err := parseChatID(chatID)
	if err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, cid, actor); err != nil {
		return nil, err
	}
	members, err := s.chats.GetChatMembers(ctx, cid)
	if err != nil {
		return nil, apperr.Internal("list members", err)
	}
	out := make([]dto.ChatMember, 0, len(members))
	for _, m := range members {
		out = append(out, dto.ChatMember{
			UserID:   string(m.UserID),
			Role:     string(m.Role),
			JoinedAt: m.JoinedAt,
		})
	}
	return &dto.ChatMembersResponse{Members: out}, nil
}

// AddMembers adds users to a group chat (owner/admin only).
func (s *ChatService) AddMembers(ctx context.Context, actor domain.ID, chatID string, in dto.AddChatMembersRequest) (*dto.ChatMembersResponse, error) {
	if actor == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	cid, err := parseChatID(chatID)
	if err != nil {
		return nil, err
	}
	c, err := s.chats.GetByID(ctx, cid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("chat not found")
		}
		return nil, apperr.Internal("load chat", err)
	}
	if c.IsDeleted {
		return nil, apperr.NotFound("chat not found")
	}
	if c.Type != domain.ChatTypeGroup {
		return nil, apperr.Forbidden("only group chats accept new members")
	}
	actorMem, err := s.requireMemberShip(ctx, cid, actor)
	if err != nil {
		return nil, err
	}
	if actorMem.Role != domain.ChatMemberRoleOwner && actorMem.Role != domain.ChatMemberRoleAdmin {
		return nil, apperr.Forbidden("insufficient role to add members")
	}

	var adds []repository.AddChatMember
	seen := map[domain.ID]struct{}{}
	for _, raw := range in.UserIDs {
		id := strings.TrimSpace(raw)
		if id == "" || !chatUUIDRe.MatchString(id) {
			return nil, apperr.Validation("invalid user_ids entry", nil)
		}
		uid := domain.ID(id)
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if _, err := s.users.GetByID(ctx, uid); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, apperr.NotFound("user not found")
			}
			return nil, apperr.Internal("load user", err)
		}
		adds = append(adds, repository.AddChatMember{UserID: uid, Role: domain.ChatMemberRoleMember})
	}
	if len(adds) == 0 {
		return nil, apperr.Validation("user_ids is required", nil)
	}
	if err := s.chats.AddMembers(ctx, cid, adds); err != nil {
		return nil, apperr.Internal("add members", err)
	}
	return s.Members(ctx, actor, chatID)
}

// RemoveMember removes a member or allows self-leave; direct chats only allow leaving yourself.
func (s *ChatService) RemoveMember(ctx context.Context, actor domain.ID, chatID, targetUserID string) error {
	if actor == "" {
		return apperr.Unauthorized("missing user")
	}
	cid, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	tid, err := parseUserIDParam(targetUserID)
	if err != nil {
		return err
	}

	c, err := s.chats.GetByID(ctx, cid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("chat not found")
		}
		return apperr.Internal("load chat", err)
	}
	if c.IsDeleted {
		return apperr.NotFound("chat not found")
	}

	actorMem, err := s.requireMemberShip(ctx, cid, actor)
	if err != nil {
		return err
	}
	targetMem, err := s.chats.FindMembership(ctx, cid, tid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("member not found")
		}
		return apperr.Internal("lookup target member", err)
	}

	if actor == tid {
		if err := s.chats.RemoveMember(ctx, cid, tid); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return apperr.NotFound("member not found")
			}
			return apperr.Internal("remove member", err)
		}
		return nil
	}

	if c.Type == domain.ChatTypeDirect {
		return apperr.Forbidden("cannot remove other user from direct chat")
	}

	if err := assertKickRules(actorMem, targetMem); err != nil {
		return err
	}
	if err := s.chats.RemoveMember(ctx, cid, tid); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("member not found")
		}
		return apperr.Internal("remove member", err)
	}
	return nil
}

func assertKickRules(actor *domain.ChatMember, target *domain.ChatMember) error {
	switch actor.Role {
	case domain.ChatMemberRoleOwner:
		if target.Role == domain.ChatMemberRoleOwner {
			return apperr.Forbidden("cannot remove owner")
		}
		return nil
	case domain.ChatMemberRoleAdmin:
		if target.Role == domain.ChatMemberRoleMember {
			return nil
		}
		return apperr.Forbidden("admins can only remove members")
	default:
		return apperr.Forbidden("insufficient role")
	}
}

func (s *ChatService) chatDetail(_ context.Context, c *domain.Chat) (*dto.ChatDetail, error) {
	if c == nil {
		return nil, apperr.Internal("nil chat", errors.New("nil chat"))
	}
	return &dto.ChatDetail{
		ID:        string(c.ID),
		Type:      string(c.Type),
		Title:     c.Title,
		AvatarURL: c.AvatarURL,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}, nil
}

func toChatSummary(row repository.ChatListRow) dto.ChatSummary {
	c := row.Chat
	sum := dto.ChatSummary{
		ID:          string(c.ID),
		Type:        string(c.Type),
		Title:       c.Title,
		AvatarURL:   c.AvatarURL,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		UnreadCount: row.UnreadCount,
	}
	if c.LastMessageID != nil && row.LastPreviewAt != nil && row.LastPreviewSender != nil && row.LastPreviewType != nil {
		sum.LastMessage = &dto.LastMessagePreview{
			ID:        string(*c.LastMessageID),
			SenderID:  string(*row.LastPreviewSender),
			Type:      string(*row.LastPreviewType),
			Text:      row.LastPreviewText,
			CreatedAt: *row.LastPreviewAt,
		}
	}
	return sum
}

func (s *ChatService) requireMember(ctx context.Context, chatID, userID domain.ID) error {
	_, err := s.chats.FindMembership(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.Forbidden("not a chat member")
		}
		return apperr.Internal("lookup membership", err)
	}
	return nil
}

func (s *ChatService) requireMemberShip(ctx context.Context, chatID, userID domain.ID) (*domain.ChatMember, error) {
	m, err := s.chats.FindMembership(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.Forbidden("not a chat member")
		}
		return nil, apperr.Internal("lookup membership", err)
	}
	return m, nil
}

func parseChatID(raw string) (domain.ID, error) {
	raw = strings.TrimSpace(raw)
	if !chatUUIDRe.MatchString(raw) {
		return "", apperr.Validation("invalid chat_id", nil)
	}
	return domain.ID(raw), nil
}

func parseUserIDParam(raw string) (domain.ID, error) {
	raw = strings.TrimSpace(raw)
	if !chatUUIDRe.MatchString(raw) {
		return "", apperr.Validation("invalid user_id", nil)
	}
	return domain.ID(raw), nil
}

func directKey(a, b domain.ID) (string, error) {
	s1, s2 := string(a), string(b)
	if s1 == "" || s2 == "" {
		return "", apperr.Validation("invalid user id", nil)
	}
	if s1 > s2 {
		s1, s2 = s2, s1
	}
	sum := sha256.Sum256([]byte(s1 + "|" + s2))
	return hex.EncodeToString(sum[:]), nil
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
