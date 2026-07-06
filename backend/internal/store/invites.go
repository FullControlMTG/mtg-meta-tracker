package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const inviteCols = `id, email, role, token_hash, invited_by, expires_at, accepted_at, created_at`

func scanInvite(row pgx.Row) (*domain.Invite, error) {
	var i domain.Invite
	err := row.Scan(&i.ID, &i.Email, &i.Role, &i.TokenHash, &i.InvitedBy,
		&i.ExpiresAt, &i.AcceptedAt, &i.CreatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &i, nil
}

func (s *Store) CreateInvite(ctx context.Context, inv *domain.Invite) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO invites (email, role, token_hash, invited_by, expires_at)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		inv.Email, inv.Role, inv.TokenHash, inv.InvitedBy, inv.ExpiresAt,
	).Scan(&inv.ID, &inv.CreatedAt)
}

// GetOpenInviteByTokenHash returns a not-yet-accepted, non-expired invite.
func (s *Store) GetOpenInviteByTokenHash(ctx context.Context, tokenHash string) (*domain.Invite, error) {
	return scanInvite(s.pool.QueryRow(ctx, `SELECT `+inviteCols+`
		FROM invites WHERE token_hash=$1 AND accepted_at IS NULL AND expires_at > now()`, tokenHash))
}

func (s *Store) ListPendingInvites(ctx context.Context) ([]domain.Invite, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+inviteCols+`
		FROM invites WHERE accepted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invite
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *Store) MarkInviteAccepted(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE invites SET accepted_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) DeleteInvite(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM invites WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
