package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const cubeCols = `id, name, moxfield_public_id, description, last_synced_at, created_at`

func scanCube(row pgx.Row) (*domain.Cube, error) {
	var c domain.Cube
	err := row.Scan(&c.ID, &c.Name, &c.MoxfieldPublicID, &c.Description, &c.LastSyncedAt, &c.CreatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &c, nil
}

func (s *Store) CreateCube(ctx context.Context, c *domain.Cube) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO cubes (name, moxfield_public_id, description)
		VALUES ($1,$2,$3) RETURNING id, created_at`,
		c.Name, c.MoxfieldPublicID, c.Description,
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
		UPDATE cubes SET name=$2, moxfield_public_id=$3, description=$4 WHERE id=$1`,
		c.ID, c.Name, c.MoxfieldPublicID, c.Description)
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

func (s *Store) SetCubeSynced(ctx context.Context, id uuid.UUID, t time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE cubes SET last_synced_at=$2 WHERE id=$1`, id, t)
	return err
}

// CountActiveCubeCards returns how many cards are currently in the pool.
func (s *Store) CountActiveCubeCards(ctx context.Context, cubeID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM cube_cards WHERE cube_id=$1 AND is_active`, cubeID).Scan(&n)
	return n, err
}
