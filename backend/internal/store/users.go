package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const userCols = `id, username, email, display_name, bio, avatar_url, role, password_hash, created_at, updated_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Bio,
		&u.AvatarURL, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO users (username, email, display_name, bio, avatar_url, role, password_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at, updated_at`,
		u.Username, u.Email, u.DisplayName, u.Bio, u.AvatarURL, u.Role, u.PasswordHash,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(username)=lower($1)`, username))
}

func (s *Store) GetUserByLogin(ctx context.Context, login string) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+`
		FROM users WHERE lower(email)=lower($1) OR lower(username)=lower($1)`, login))
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// Writes role too; the caller must admin-gate role changes.
func (s *Store) UpdateUserProfile(ctx context.Context, u *domain.User) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE users SET display_name=$2, bio=$3, avatar_url=$4, role=$5, updated_at=now()
		WHERE id=$1`, u.ID, u.DisplayName, u.Bio, u.AvatarURL, u.Role)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}
