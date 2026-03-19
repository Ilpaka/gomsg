package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"goflow/backend/internal/config"
	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	"goflow/backend/internal/pkg/auth"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/repository"
)

var (
	nicknameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailRe    = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

// ClientMeta carries optional client metadata for refresh sessions.
type ClientMeta struct {
	UserAgent string
	IP        string
}

// AuthService implements registration, login, token refresh, and logout flows.
type AuthService struct {
	users    repository.UserRepository
	sessions repository.SessionRepository
	cfg      *config.Config
}

func NewAuthService(
	users repository.UserRepository,
	sessions repository.SessionRepository,
	cfg *config.Config,
) *AuthService {
	return &AuthService{users: users, sessions: sessions, cfg: cfg}
}

func (s *AuthService) Register(ctx context.Context, in dto.RegisterRequest, meta ClientMeta) (*dto.RegisterResponse, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	nick := strings.TrimSpace(in.Nickname)
	if err := validateCredentials(email, nick, in.Password); err != nil {
		return nil, err
	}

	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, apperr.Conflict("email already registered")
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.Internal("lookup user by email", err)
	}
	if _, err := s.users.GetByNickname(ctx, nick); err == nil {
		return nil, apperr.Conflict("nickname already taken")
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.Internal("lookup user by nickname", err)
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, apperr.Validation("password", err)
	}

	u := &domain.User{
		Email:        email,
		PasswordHash: hash,
		Nickname:     nick,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, apperr.Internal("create user", err)
	}

	tokens, err := s.issueTokens(ctx, u.ID, meta)
	if err != nil {
		return nil, err
	}

	return &dto.RegisterResponse{
		User: dto.UserPublic{
			ID:       string(u.ID),
			Email:    u.Email,
			Nickname: u.Nickname,
		},
		Tokens: *tokens,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, in dto.LoginRequest, meta ClientMeta) (*dto.LoginResponse, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if email == "" || in.Password == "" {
		return nil, apperr.Validation("email and password are required", nil)
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.Unauthorized("invalid credentials")
		}
		return nil, apperr.Internal("lookup user", err)
	}
	if !u.IsActive {
		return nil, apperr.Forbidden("account disabled")
	}
	if err := auth.ComparePassword(u.PasswordHash, in.Password); err != nil {
		return nil, apperr.Unauthorized("invalid credentials")
	}

	tokens, err := s.issueTokens(ctx, u.ID, meta)
	if err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		User: dto.UserPublic{
			ID:       string(u.ID),
			Email:    u.Email,
			Nickname: u.Nickname,
		},
		Tokens: *tokens,
	}, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshPlain string) (*dto.RefreshResponse, error) {
	refreshPlain = strings.TrimSpace(refreshPlain)
	if refreshPlain == "" {
		return nil, apperr.Validation("refresh_token is required", nil)
	}

	h := hashRefreshToken(refreshPlain)
	sess, err := s.sessions.GetByTokenHash(ctx, h)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, apperr.Unauthorized("invalid refresh token")
		}
		return nil, apperr.Internal("lookup session", err)
	}

	access, err := s.newAccessToken(string(sess.UserID))
	if err != nil {
		return nil, err
	}

	return &dto.RefreshResponse{
		AccessToken: access,
		ExpiresIn:   s.cfg.JWT.AccessTTLSeconds,
		TokenType:   "Bearer",
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshPlain string) error {
	refreshPlain = strings.TrimSpace(refreshPlain)
	if refreshPlain == "" {
		return apperr.Validation("refresh_token is required", nil)
	}
	h := hashRefreshToken(refreshPlain)
	sess, err := s.sessions.GetByTokenHash(ctx, h)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.Unauthorized("invalid refresh token")
		}
		return apperr.Internal("lookup session", err)
	}
	if err := s.sessions.Revoke(ctx, sess.ID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.Unauthorized("invalid refresh token")
		}
		return apperr.Internal("revoke session", err)
	}
	return nil
}

func (s *AuthService) LogoutAll(ctx context.Context, userID domain.ID) error {
	if userID == "" {
		return apperr.Unauthorized("missing user")
	}
	if err := s.sessions.RevokeAllByUser(ctx, userID); err != nil {
		return apperr.Internal("revoke sessions", err)
	}
	return nil
}

func (s *AuthService) issueTokens(ctx context.Context, userID domain.ID, meta ClientMeta) (*dto.TokenPair, error) {
	access, err := s.newAccessToken(string(userID))
	if err != nil {
		return nil, err
	}

	plain, err := newRefreshPlain()
	if err != nil {
		return nil, apperr.Internal("generate refresh token", err)
	}
	hash := hashRefreshToken(plain)

	ttl := time.Duration(s.cfg.JWT.RefreshTTLSeconds) * time.Second
	if ttl <= 0 {
		return nil, apperr.Internal("invalid refresh ttl", fmt.Errorf("refresh_ttl_seconds=%d", s.cfg.JWT.RefreshTTLSeconds))
	}
	exp := time.Now().UTC().Add(ttl)

	sess := &domain.RefreshSession{
		UserID:    userID,
		TokenHash: hash,
		UserAgent: strOrNil(meta.UserAgent),
		IPAddress: strOrNil(trimIP(meta.IP)),
		ExpiresAt: exp,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, apperr.Internal("create refresh session", err)
	}

	return &dto.TokenPair{
		AccessToken:  access,
		RefreshToken: plain,
		ExpiresIn:    s.cfg.JWT.AccessTTLSeconds,
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) newAccessToken(userID string) (string, error) {
	secret := []byte(strings.TrimSpace(s.cfg.JWT.Secret))
	ttl := time.Duration(s.cfg.JWT.AccessTTLSeconds) * time.Second
	tok, err := auth.GenerateAccessToken(secret, userID, ttl)
	if err != nil {
		return "", apperr.Internal("sign access token", err)
	}
	return tok, nil
}

func validateCredentials(email, nickname, password string) error {
	if email == "" || nickname == "" || password == "" {
		return apperr.Validation("email, nickname and password are required", nil)
	}
	if len(email) > 320 {
		return apperr.Validation("email too long", nil)
	}
	if !emailRe.MatchString(email) {
		return apperr.Validation("invalid email format", nil)
	}
	if !nicknameRe.MatchString(nickname) {
		return apperr.Validation("nickname must be 3-32 chars: letters, digits, underscore", nil)
	}
	if len(password) < 8 || len(password) > 128 {
		return apperr.Validation("password length must be 8-128", nil)
	}
	return nil
}

func hashRefreshToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func newRefreshPlain() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func strOrNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func trimIP(ip string) string {
	ip = strings.TrimSpace(ip)
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		ip = host
	}
	if len(ip) > 45 {
		return ip[:45]
	}
	return ip
}
