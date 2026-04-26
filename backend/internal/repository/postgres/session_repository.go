package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// SessionRepository implements repository.SessionRepository using pgx.
type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

var _ repository.SessionRepository = (*SessionRepository)(nil)

func (r *SessionRepository) Create(ctx context.Context, s *domain.RefreshSession) error {
	const q = `
INSERT INTO refresh_sessions (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1::uuid, $2, $3, $4, $5)
RETURNING id::text, user_id::text, token_hash, user_agent, ip_address, expires_at, created_at, revoked_at`

	row := r.pool.QueryRow(ctx, q,
		string(s.UserID),
		s.TokenHash,
		s.UserAgent,
		s.IPAddress,
		s.ExpiresAt,
	)
	out, err := scanSession(row)
	if err != nil {
		return err
	}
	*s = out
	return nil
}

func (r *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error) {
	const q = `
SELECT id::text, user_id::text, token_hash, user_agent, ip_address, expires_at, created_at, revoked_at
FROM refresh_sessions
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > $2`

	row := r.pool.QueryRow(ctx, q, tokenHash, time.Now().UTC())
	s, err := scanSession(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &s, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, id domain.ID) error {
	const q = `
UPDATE refresh_sessions
SET revoked_at = now()
WHERE id = $1::uuid AND revoked_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, string(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) RevokeAllByUser(ctx context.Context, userID domain.ID) error {
	const q = `
UPDATE refresh_sessions
SET revoked_at = now()
WHERE user_id = $1::uuid AND revoked_at IS NULL`

	_, err := r.pool.Exec(ctx, q, string(userID))
	return err
}

func (r *SessionRepository) RotateRefresh(ctx context.Context, oldTokenHash string, newSess *domain.RefreshSession) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldID string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM refresh_sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
FOR UPDATE`, oldTokenHash).Scan(&oldID)
	if err != nil {
		return mapErr(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = now() WHERE id = $1::uuid`, oldID); err != nil {
		return err
	}

	const ins = `
INSERT INTO refresh_sessions (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1::uuid, $2, $3, $4, $5)
RETURNING id::text, user_id::text, token_hash, user_agent, ip_address, expires_at, created_at, revoked_at`
	row := tx.QueryRow(ctx, ins,
		string(newSess.UserID),
		newSess.TokenHash,
		newSess.UserAgent,
		newSess.IPAddress,
		newSess.ExpiresAt,
	)
	out, err := scanSession(row)
	if err != nil {
		return err
	}
	*newSess = out
	return tx.Commit(ctx)
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(row sessionScanner) (domain.RefreshSession, error) {
	var s domain.RefreshSession
	var id, userID string
	err := row.Scan(
		&id,
		&userID,
		&s.TokenHash,
		&s.UserAgent,
		&s.IPAddress,
		&s.ExpiresAt,
		&s.CreatedAt,
		&s.RevokedAt,
	)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	s.ID = domain.ID(id)
	s.UserID = domain.ID(userID)
	return s, nil
}
