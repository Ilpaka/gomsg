package repository

import (
	"context"

	"goflow/backend/internal/domain"
)

// SessionRepository persists refresh token sessions.
type SessionRepository interface {
	Create(ctx context.Context, s *domain.RefreshSession) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error)
	Revoke(ctx context.Context, id domain.ID) error
	RevokeAllByUser(ctx context.Context, userID domain.ID) error
	// RotateRefresh revokes the session matching oldHash and inserts newSess in one transaction.
	RotateRefresh(ctx context.Context, oldTokenHash string, newSess *domain.RefreshSession) error
}
