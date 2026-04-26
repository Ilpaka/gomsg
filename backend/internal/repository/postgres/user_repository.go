package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/repository"
)

// UserRepository implements repository.UserRepository using pgx.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

var _ repository.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	const q = `
INSERT INTO users (email, password_hash, nickname, first_name, last_name, avatar_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, email, password_hash, nickname, first_name, last_name, avatar_url,
          created_at, updated_at, last_seen_at, is_active`

	row := r.pool.QueryRow(ctx, q,
		u.Email,
		u.PasswordHash,
		u.Nickname,
		u.FirstName,
		u.LastName,
		u.AvatarURL,
	)
	out, err := scanUser(row)
	if err != nil {
		return err
	}
	*u = out
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id domain.ID) (*domain.User, error) {
	const q = `
SELECT id::text, email, password_hash, nickname, first_name, last_name, avatar_url,
       created_at, updated_at, last_seen_at, is_active
FROM users
WHERE id = $1::uuid`

	row := r.pool.QueryRow(ctx, q, string(id))
	u, err := scanUser(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `
SELECT id::text, email, password_hash, nickname, first_name, last_name, avatar_url,
       created_at, updated_at, last_seen_at, is_active
FROM users
WHERE lower(email) = lower($1)`

	row := r.pool.QueryRow(ctx, q, email)
	u, err := scanUser(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepository) GetByNickname(ctx context.Context, nickname string) (*domain.User, error) {
	const q = `
SELECT id::text, email, password_hash, nickname, first_name, last_name, avatar_url,
       created_at, updated_at, last_seen_at, is_active
FROM users
WHERE lower(nickname) = lower($1)`

	row := r.pool.QueryRow(ctx, q, nickname)
	u, err := scanUser(row)
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepository) Search(ctx context.Context, query string, page repository.Page) ([]domain.User, error) {
	limit, offset := normPage(page)
	pattern := "%" + strings.TrimSpace(query) + "%"
	if pattern == "%%" {
		pattern = "%"
	}

	const q = `
SELECT id::text, email, nickname, first_name, last_name, avatar_url,
       created_at, updated_at, last_seen_at, is_active
FROM users
WHERE is_active = true
  AND (
    nickname ILIKE $1
    OR COALESCE(first_name, '') ILIKE $1
    OR COALESCE(last_name, '') ILIKE $1
  )
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, pattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.User
	for rows.Next() {
		u, err := scanUserSearchRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepository) UpdateProfile(ctx context.Context, p repository.UpdateUserProfileParams) error {
	const q = `
UPDATE users
SET nickname = COALESCE($2, nickname),
    first_name = COALESCE($3, first_name),
    last_name = COALESCE($4, last_name),
    avatar_url = COALESCE($5, avatar_url),
    updated_at = now()
WHERE id = $1::uuid`

	tag, err := r.pool.Exec(ctx, q,
		string(p.UserID),
		p.Nickname,
		p.FirstName,
		p.LastName,
		p.AvatarURL,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (domain.User, error) {
	var u domain.User
	var id string
	err := row.Scan(
		&id,
		&u.Email,
		&u.PasswordHash,
		&u.Nickname,
		&u.FirstName,
		&u.LastName,
		&u.AvatarURL,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastSeenAt,
		&u.IsActive,
	)
	if err != nil {
		return domain.User{}, err
	}
	u.ID = domain.ID(id)
	return u, nil
}

func scanUserSearchRow(row userScanner) (domain.User, error) {
	var u domain.User
	var id string
	err := row.Scan(
		&id,
		&u.Email,
		&u.Nickname,
		&u.FirstName,
		&u.LastName,
		&u.AvatarURL,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastSeenAt,
		&u.IsActive,
	)
	if err != nil {
		return domain.User{}, err
	}
	u.ID = domain.ID(id)
	u.PasswordHash = ""
	return u, nil
}
