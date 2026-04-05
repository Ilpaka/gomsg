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

var (
	userNicknameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	userUUIDRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

const (
	userSearchDefaultLimit = 20
	userSearchMaxLimit     = 50
	userNameMaxLen         = 100
	userAvatarMaxLen       = 2048
)

// UserService handles user profile reads and updates.
type UserService struct {
	users repository.UserRepository
}

func NewUserService(users repository.UserRepository) *UserService {
	return &UserService{users: users}
}

func (s *UserService) Me(ctx context.Context, userID domain.ID) (*dto.UserSelf, error) {
	if userID == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("load user", err)
	}
	return toUserSelf(u), nil
}

func (s *UserService) UpdateMe(ctx context.Context, userID domain.ID, in dto.PatchUserRequest) (*dto.UserSelf, error) {
	if userID == "" {
		return nil, apperr.Unauthorized("missing user")
	}
	if in.Nickname == nil && in.FirstName == nil && in.LastName == nil && in.AvatarURL == nil {
		return nil, apperr.Validation("no fields to update", nil)
	}

	if in.Nickname != nil {
		nick := strings.TrimSpace(*in.Nickname)
		if !userNicknameRe.MatchString(nick) {
			return nil, apperr.Validation("nickname must be 3-32 chars: letters, digits, underscore", nil)
		}
		other, err := s.users.GetByNickname(ctx, nick)
		if err == nil && other.ID != userID {
			return nil, apperr.Conflict("nickname already taken")
		}
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.Internal("lookup nickname", err)
		}
		in.Nickname = ptrString(nick)
	}
	if in.FirstName != nil {
		v := strings.TrimSpace(*in.FirstName)
		if len(v) > userNameMaxLen {
			return nil, apperr.Validation("first_name too long", nil)
		}
		in.FirstName = ptrString(v)
	}
	if in.LastName != nil {
		v := strings.TrimSpace(*in.LastName)
		if len(v) > userNameMaxLen {
			return nil, apperr.Validation("last_name too long", nil)
		}
		in.LastName = ptrString(v)
	}
	if in.AvatarURL != nil {
		v := strings.TrimSpace(*in.AvatarURL)
		if len(v) > userAvatarMaxLen {
			return nil, apperr.Validation("avatar_url too long", nil)
		}
		in.AvatarURL = ptrString(v)
	}

	p := repository.UpdateUserProfileParams{UserID: userID}
	if in.Nickname != nil {
		p.Nickname = in.Nickname
	}
	if in.FirstName != nil {
		p.FirstName = in.FirstName
	}
	if in.LastName != nil {
		p.LastName = in.LastName
	}
	if in.AvatarURL != nil {
		p.AvatarURL = in.AvatarURL
	}

	if err := s.users.UpdateProfile(ctx, p); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("update profile", err)
	}

	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, apperr.Internal("reload user", err)
	}
	return toUserSelf(u), nil
}

func (s *UserService) Search(ctx context.Context, q string, limit, offset int) (*dto.UserSearchResponse, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, apperr.Validation("q is required", nil)
	}
	if limit <= 0 {
		limit = userSearchDefaultLimit
	}
	if limit > userSearchMaxLimit {
		limit = userSearchMaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.users.Search(ctx, q, repository.Page{Limit: limit, Offset: offset})
	if err != nil {
		return nil, apperr.Internal("search users", err)
	}
	out := make([]dto.UserCard, 0, len(rows))
	for i := range rows {
		out = append(out, *toUserCard(&rows[i]))
	}
	return &dto.UserSearchResponse{Users: out}, nil
}

func (s *UserService) GetByID(ctx context.Context, id string) (*dto.UserCard, error) {
	id = strings.TrimSpace(id)
	if !userUUIDRe.MatchString(id) {
		return nil, apperr.Validation("invalid user id", nil)
	}
	u, err := s.users.GetByID(ctx, domain.ID(id))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("load user", err)
	}
	return toUserCard(u), nil
}

func toUserSelf(u *domain.User) *dto.UserSelf {
	if u == nil {
		return nil
	}
	return &dto.UserSelf{
		ID:        string(u.ID),
		Email:     u.Email,
		Nickname:  u.Nickname,
		FirstName: copyStrPtr(u.FirstName),
		LastName:  copyStrPtr(u.LastName),
		AvatarURL: copyStrPtr(u.AvatarURL),
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		LastSeen:  copyTimePtr(u.LastSeenAt),
	}
}

func toUserCard(u *domain.User) *dto.UserCard {
	if u == nil {
		return nil
	}
	return &dto.UserCard{
		ID:        string(u.ID),
		Nickname:  u.Nickname,
		FirstName: copyStrPtr(u.FirstName),
		LastName:  copyStrPtr(u.LastName),
		AvatarURL: copyStrPtr(u.AvatarURL),
		IsActive:  u.IsActive,
	}
}

func ptrString(s string) *string {
	return &s
}

func copyStrPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}
