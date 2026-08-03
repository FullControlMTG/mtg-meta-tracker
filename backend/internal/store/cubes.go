package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const cubeCols = `id, name, owner_id, moxfield_public_id, description, card_list, content_hash, last_synced_at, created_at`

func scanCube(row pgx.Row) (*domain.Cube, error) {
	var c domain.Cube
	err := row.Scan(&c.ID, &c.Name, &c.OwnerID, &c.MoxfieldPublicID, &c.Description, &c.CardList, &c.ContentHash, &c.LastSyncedAt, &c.CreatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &c, nil
}

// CreateCube inserts the cube and enrolls its owner as the first member, so the
// creator can see what they just made without inviting themselves.
func (s *Store) CreateCube(ctx context.Context, c *domain.Cube) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.QueryRow(ctx, `
		INSERT INTO cubes (name, owner_id, moxfield_public_id, description, card_list)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		c.Name, c.OwnerID, c.MoxfieldPublicID, c.Description, c.CardList,
	).Scan(&c.ID, &c.CreatedAt); err != nil {
		return err
	}
	if c.OwnerID != nil {
		if _, err := tx.Exec(ctx,
			`INSERT INTO cube_members (cube_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			c.ID, *c.OwnerID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) GetCube(ctx context.Context, id uuid.UUID) (*domain.Cube, error) {
	return scanCube(s.pool.QueryRow(ctx, `SELECT `+cubeCols+` FROM cubes WHERE id=$1`, id))
}

// ListCubes returns every cube, unscoped. For the scheduler (which must sync all
// pools) and internal lookups — never a user-facing list; that is ListCubesForUser.
func (s *Store) ListCubes(ctx context.Context) ([]domain.Cube, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cubeCols+` FROM cubes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Cube
	for rows.Next() {
		c, err := scanCube(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ListCubesForUser returns the cubes a caller may see: an admin sees all, everyone
// else only the cubes they are a member of.
func (s *Store) ListCubesForUser(ctx context.Context, userID uuid.UUID, isAdmin bool) ([]domain.Cube, error) {
	q := `SELECT ` + cubeCols + ` FROM cubes c ORDER BY created_at`
	args := []any{}
	if !isAdmin {
		q = `SELECT ` + cubeCols + ` FROM cubes c
			WHERE EXISTS (SELECT 1 FROM cube_members m WHERE m.cube_id = c.id AND m.user_id = $1)
			ORDER BY created_at`
		args = append(args, userID)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Cube
	for rows.Next() {
		c, err := scanCube(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateCube(ctx context.Context, c *domain.Cube) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE cubes SET name=$2, moxfield_public_id=$3, description=$4, card_list=$5 WHERE id=$1`,
		c.ID, c.Name, c.MoxfieldPublicID, c.Description, c.CardList)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCube(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM cubes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCubeSyncState records the fingerprint of the last successfully synced
// Moxfield list along with the sync timestamp. It deliberately leaves the other
// cube columns untouched so an admin PATCH cannot race/clobber the hash.
func (s *Store) SetCubeSyncState(ctx context.Context, id uuid.UUID, hash string, t time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE cubes SET content_hash=$2, last_synced_at=$3 WHERE id=$1`, id, hash, t)
	return err
}

// ClearCubeContentHash nulls the change-detection fingerprint so the next
// SyncCube re-resolves the pool even if the card list is unchanged. Used by the
// admin "Sync Scryfall images" action to retry names that previously failed to resolve.
func (s *Store) ClearCubeContentHash(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE cubes SET content_hash=NULL WHERE id=$1`, id)
	return err
}

// CountActiveCubeCards counts the cube's distinct printings — one per cube_cards
// row, which is what a resolve produces and what sync progress is measured in.
func (s *Store) CountActiveCubeCards(ctx context.Context, cubeID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM cube_cards WHERE cube_id=$1 AND is_active`, cubeID).Scan(&n)
	return n, err
}

// CountActiveCubeCopies counts physical cards: 150 Ornithopters are 150. This is the
// "how big is this cube" number the cube pages show; for a singleton cube — nearly all
// of them — it is identical to CountActiveCubeCards.
func (s *Store) CountActiveCubeCopies(ctx context.Context, cubeID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT coalesce(sum(quantity), 0) FROM cube_cards WHERE cube_id=$1 AND is_active`,
		cubeID).Scan(&n)
	return n, err
}

// --- membership ---

func (s *Store) IsCubeMember(ctx context.Context, cubeID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM cube_members WHERE cube_id=$1 AND user_id=$2)`,
		cubeID, userID).Scan(&ok)
	return ok, err
}

func (s *Store) IsCubeOwner(ctx context.Context, cubeID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM cubes WHERE id=$1 AND owner_id=$2)`,
		cubeID, userID).Scan(&ok)
	return ok, err
}

// ListCubeMembers returns a cube's members as full user rows, ordered by join time,
// for the owner picker and the members admin panel.
func (s *Store) ListCubeMembers(ctx context.Context, cubeID uuid.UUID) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		u.id, u.username, u.email, u.display_name, u.bio, u.avatar_url, u.role, u.password_hash, u.created_at, u.updated_at
		FROM cube_members m JOIN users u ON u.id = m.user_id
		WHERE m.cube_id=$1 ORDER BY m.joined_at`, cubeID)
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

// RemoveCubeMember drops a member. Removing the owner is refused — ownership is
// transferred, not abandoned.
func (s *Store) RemoveCubeMember(ctx context.Context, cubeID, userID uuid.UUID) error {
	owner, err := s.IsCubeOwner(ctx, cubeID, userID)
	if err != nil {
		return err
	}
	if owner {
		return ErrConflict
	}
	ct, err := s.pool.Exec(ctx, `DELETE FROM cube_members WHERE cube_id=$1 AND user_id=$2`, cubeID, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- invites ---

// CreateInvite records a pending invite. Re-inviting a user who declined (or was
// removed after accepting) resets their existing row to pending rather than erroring
// on the UNIQUE(cube_id, invitee_id) constraint.
func (s *Store) CreateInvite(ctx context.Context, cubeID, inviteeID, invitedBy uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO cube_invites (cube_id, invitee_id, invited_by, status)
		VALUES ($1,$2,$3,'pending')
		ON CONFLICT (cube_id, invitee_id)
		DO UPDATE SET status='pending', invited_by=$3, created_at=now(), responded_at=NULL
		RETURNING id`, cubeID, inviteeID, invitedBy).Scan(&id)
	return id, err
}

// CubeInviteView is a pending invite joined to the names a UI needs, so neither the
// dashboard nor the members panel has to look users/cubes up per row.
type CubeInviteView struct {
	ID          uuid.UUID `json:"id"`
	CubeID      uuid.UUID `json:"cube_id"`
	CubeName    string    `json:"cube_name"`
	InviteeID   uuid.UUID `json:"invitee_id"`
	InviteeName string    `json:"invitee_name"`
	InvitedBy   *string   `json:"invited_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListUserInvites returns a user's pending invites, for their dashboard.
func (s *Store) ListUserInvites(ctx context.Context, userID uuid.UUID) ([]CubeInviteView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.cube_id, c.name, i.invitee_id, invitee.display_name,
		       inviter.display_name, i.created_at
		FROM cube_invites i
		JOIN cubes c ON c.id = i.cube_id
		JOIN users invitee ON invitee.id = i.invitee_id
		LEFT JOIN users inviter ON inviter.id = i.invited_by
		WHERE i.invitee_id=$1 AND i.status='pending'
		ORDER BY i.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanInviteViews(rows)
}

// ListCubeInvites returns a cube's outstanding invites, for the owner's members panel.
func (s *Store) ListCubeInvites(ctx context.Context, cubeID uuid.UUID) ([]CubeInviteView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.cube_id, c.name, i.invitee_id, invitee.display_name,
		       inviter.display_name, i.created_at
		FROM cube_invites i
		JOIN cubes c ON c.id = i.cube_id
		JOIN users invitee ON invitee.id = i.invitee_id
		LEFT JOIN users inviter ON inviter.id = i.invited_by
		WHERE i.cube_id=$1 AND i.status='pending'
		ORDER BY i.created_at DESC`, cubeID)
	if err != nil {
		return nil, err
	}
	return scanInviteViews(rows)
}

func scanInviteViews(rows pgx.Rows) ([]CubeInviteView, error) {
	defer rows.Close()
	out := []CubeInviteView{}
	for rows.Next() {
		var v CubeInviteView
		if err := rows.Scan(&v.ID, &v.CubeID, &v.CubeName, &v.InviteeID, &v.InviteeName,
			&v.InvitedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetInviteRecipient returns the invitee a pending invite is addressed to, so a
// handler can confirm the caller is the one accepting/declining it.
func (s *Store) GetInviteRecipient(ctx context.Context, inviteID uuid.UUID) (uuid.UUID, error) {
	var invitee uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT invitee_id FROM cube_invites WHERE id=$1 AND status='pending'`, inviteID).Scan(&invitee)
	if err != nil {
		return uuid.Nil, normErr(err)
	}
	return invitee, nil
}

// AcceptInvite enrolls the invitee and marks the invite accepted, in one transaction.
func (s *Store) AcceptInvite(ctx context.Context, inviteID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var cubeID, inviteeID uuid.UUID
	if err := tx.QueryRow(ctx, `
		UPDATE cube_invites SET status='accepted', responded_at=now()
		WHERE id=$1 AND status='pending'
		RETURNING cube_id, invitee_id`, inviteID).Scan(&cubeID, &inviteeID); err != nil {
		return normErr(err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO cube_members (cube_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		cubeID, inviteeID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeclineInvite(ctx context.Context, inviteID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE cube_invites SET status='declined', responded_at=now() WHERE id=$1 AND status='pending'`,
		inviteID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
