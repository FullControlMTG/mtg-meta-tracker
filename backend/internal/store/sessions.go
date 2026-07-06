package store

import (
	"context"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

func (s *Store) CreateSession(ctx context.Context, sess *domain.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, expires_at) VALUES ($1,$2,$3)`,
		sess.ID, sess.UserID, sess.ExpiresAt)
	return err
}

func (s *Store) GetValidSessionUser(ctx context.Context, sessionID string) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.email, u.display_name, u.bio, u.avatar_url,
		       u.role, u.password_hash, u.created_at, u.updated_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id=$1 AND s.expires_at > now()`, sessionID))
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, sessionID)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID any) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}
