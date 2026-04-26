package service

import (
	"context"
	"testing"

	"goflow/backend/internal/config"
	"goflow/backend/internal/domain"
	"goflow/backend/internal/dto"
	"goflow/backend/internal/pkg/auth"
	apperr "goflow/backend/internal/pkg/errors"
)

func testAuthConfig() *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			Secret:              "local-dev-only-change-in-prod-min-16",
			AccessTTLSeconds:    900,
			RefreshTTLSeconds:   604800,
		},
	}
}

func TestAuthService_Register_success(t *testing.T) {
	t.Parallel()
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, testAuthConfig())

	out, err := svc.Register(context.Background(), dto.RegisterRequest{
		Email:    "alice@example.com",
		Nickname: "alice_m",
		Password: "password123",
	}, ClientMeta{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if out.User.Email != "alice@example.com" || out.User.Nickname != "alice_m" {
		t.Fatalf("unexpected user: %+v", out.User)
	}
	if out.Tokens.AccessToken == "" || out.Tokens.RefreshToken == "" {
		t.Fatalf("expected tokens")
	}
}

func TestAuthService_Register_emailConflict(t *testing.T) {
	t.Parallel()
	users := newFakeUserRepo()
	users.byEmail["alice@example.com"] = &domain.User{Email: "alice@example.com"}
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, testAuthConfig())

	_, err := svc.Register(context.Background(), dto.RegisterRequest{
		Email:    "alice@example.com",
		Nickname: "bob_m",
		Password: "password123",
	}, ClientMeta{})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindConflict {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestAuthService_Login_success(t *testing.T) {
	t.Parallel()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	users.byEmail["bob@example.com"] = &domain.User{
		ID:           domain.ID("10000000-0000-4000-8000-000000000001"),
		Email:        "bob@example.com",
		Nickname:     "bob_m",
		PasswordHash: hash,
		IsActive:     true,
	}
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, testAuthConfig())

	out, err := svc.Login(context.Background(), dto.LoginRequest{
		Email:    "bob@example.com",
		Password: "password123",
	}, ClientMeta{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if out.User.ID != string(users.byEmail["bob@example.com"].ID) {
		t.Fatalf("unexpected user id %s", out.User.ID)
	}
}

func TestAuthService_Login_invalidPassword(t *testing.T) {
	t.Parallel()
	hash, err := auth.HashPassword("right-password")
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	users.byEmail["bob@example.com"] = &domain.User{
		ID:           domain.ID("10000000-0000-4000-8000-000000000001"),
		Email:        "bob@example.com",
		Nickname:     "bob_m",
		PasswordHash: hash,
		IsActive:     true,
	}
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, testAuthConfig())

	_, err = svc.Login(context.Background(), dto.LoginRequest{
		Email:    "bob@example.com",
		Password: "wrong-password",
	}, ClientMeta{})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindUnauthorized {
		t.Fatalf("want unauthorized, got %v", err)
	}
}

func TestAuthService_Refresh_rotation(t *testing.T) {
	t.Parallel()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	uid := domain.ID("10000000-0000-4000-8000-000000000001")
	users.byEmail["carol@example.com"] = &domain.User{
		ID:           uid,
		Email:        "carol@example.com",
		Nickname:     "carol_m",
		PasswordHash: hash,
		IsActive:     true,
	}
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, testAuthConfig())

	loginOut, err := svc.Login(context.Background(), dto.LoginRequest{
		Email:    "carol@example.com",
		Password: "password123",
	}, ClientMeta{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := loginOut.Tokens.RefreshToken

	ref1, err := svc.Refresh(context.Background(), oldRefresh, ClientMeta{})
	if err != nil {
		t.Fatalf("Refresh #1: %v", err)
	}
	if ref1.RefreshToken == "" || ref1.RefreshToken == oldRefresh {
		t.Fatalf("expected new refresh token, got empty or same as old")
	}

	if _, err := svc.Refresh(context.Background(), oldRefresh, ClientMeta{}); err == nil {
		t.Fatal("expected error when reusing revoked refresh token")
	}

	ref2, err := svc.Refresh(context.Background(), ref1.RefreshToken, ClientMeta{})
	if err != nil {
		t.Fatalf("Refresh #2: %v", err)
	}
	if ref2.RefreshToken == "" || ref2.RefreshToken == ref1.RefreshToken {
		t.Fatalf("expected second rotated refresh")
	}

	if _, err := svc.Refresh(context.Background(), ref1.RefreshToken, ClientMeta{}); err == nil {
		t.Fatal("expected error when reusing first rotated token after second rotation")
	}
}
