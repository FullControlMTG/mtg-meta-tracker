package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const cubeCols = `id, name, moxfield_public_id, description, card_list, content_hash, last_synced_at, created_at`

func scanCube(row pgx.Row) (*domain.Cube, error) {
	var c domain.Cube
	err := row.Scan(&c.ID, &c.Name, &c.MoxfieldPublicID, &c.Description, &c.CardList, &c.ContentHash, &c.LastSyncedAt, &c.CreatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &c, nil
}

func (s *Store) CreateCube(ctx context.Context, c *domain.Cube) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO cubes (name, moxfield_public_id, description, card_list)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		c.Name, c.MoxfieldPublicID, c.Description, c.CardList,
	).Scan(&c.ID, &c.CreatedAt)
}

func (s *Store) GetCube(ctx context.Context, id uuid.UUID) (*domain.Cube, error) {
	return scanCube(s.pool.QueryRow(ctx, `SELECT `+cubeCols+` FROM cubes WHERE id=$1`, id))
}

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
