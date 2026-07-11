package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CubeSyncProgress is the live state of the admin "Sync Scryfall images" action
// for a single cube. It advances queued → resolving → downloading → done/failed.
// The image-download phase runs detached from the job, so this row (not the
// job's status) is the source of truth for whether a sync has finished.
type CubeSyncProgress struct {
	CubeID       uuid.UUID  `json:"cube_id"`
	Status       string     `json:"status"`
	CardsTotal   int        `json:"cards_total"`
	ImagesTotal  int        `json:"images_total"`
	ImagesDone   int        `json:"images_done"`
	ImagesFailed int        `json:"images_failed"`
	Error        *string    `json:"error,omitempty"`
	Unresolved   []string   `json:"unresolved"`
	StartedAt    time.Time  `json:"started_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

const cubeSyncCols = `cube_id, status, cards_total, images_total, images_done, images_failed, error, unresolved, started_at, updated_at, finished_at`

// BeginCubeSyncProgress upserts a fresh progress row for a starting sync,
// resetting counters/error and clearing finished_at. It deliberately leaves
// `unresolved` alone: a sync that finds the list unchanged skips the resolve
// entirely, and the previous run's unresolved names are still the truth for the
// current pool. A run that does resolve overwrites it via SetCubeSyncUnresolved.
func (s *Store) BeginCubeSyncProgress(ctx context.Context, cubeID uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cube_sync_progress (cube_id, status, started_at, updated_at)
		VALUES ($1, $2, now(), now())
		ON CONFLICT (cube_id) DO UPDATE SET
			status=$2, cards_total=0, images_total=0, images_done=0, images_failed=0,
			error=NULL, started_at=now(), updated_at=now(), finished_at=NULL`,
		cubeID, status)
	return err
}

// SetCubeSyncResolved records the resolved card/image totals and moves the row
// into the downloading phase.
func (s *Store) SetCubeSyncResolved(ctx context.Context, cubeID uuid.UUID, cardsTotal, imagesTotal int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE cube_sync_progress
		SET status='downloading', cards_total=$2, images_total=$3, images_done=0, updated_at=now()
		WHERE cube_id=$1`,
		cubeID, cardsTotal, imagesTotal)
	return err
}

// SetCubeSyncUnresolved records the names Scryfall could not resolve on this
// run. Call it after every resolve, including with an empty slice, so a run that
// fixes a typo clears the previous run's list.
func (s *Store) SetCubeSyncUnresolved(ctx context.Context, cubeID uuid.UUID, names []string) error {
	if names == nil {
		names = []string{} // the column is NOT NULL; nil would encode as NULL
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE cube_sync_progress
		SET unresolved=$2, updated_at=now()
		WHERE cube_id=$1`,
		cubeID, names)
	return err
}

// SetCubeSyncImages sets the current image download counters (absolute values).
// Called throttled from the detached download goroutine.
func (s *Store) SetCubeSyncImages(ctx context.Context, cubeID uuid.UUID, done, failed int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE cube_sync_progress
		SET images_done=$2, images_failed=$3, updated_at=now()
		WHERE cube_id=$1`,
		cubeID, done, failed)
	return err
}

// FinishCubeSyncProgress marks the sync done or failed. errMsg is stored only
// for the failed status ("" clears it).
func (s *Store) FinishCubeSyncProgress(ctx context.Context, cubeID uuid.UUID, status, errMsg string) error {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE cube_sync_progress
		SET status=$2, error=$3, updated_at=now(), finished_at=now()
		WHERE cube_id=$1`,
		cubeID, status, e)
	return err
}

// GetCubeSyncProgress returns the latest progress row for a cube, or
// ErrNotFound when the cube has never been synced.
func (s *Store) GetCubeSyncProgress(ctx context.Context, cubeID uuid.UUID) (*CubeSyncProgress, error) {
	var p CubeSyncProgress
	err := s.pool.QueryRow(ctx,
		`SELECT `+cubeSyncCols+` FROM cube_sync_progress WHERE cube_id=$1`, cubeID).
		Scan(&p.CubeID, &p.Status, &p.CardsTotal, &p.ImagesTotal, &p.ImagesDone,
			&p.ImagesFailed, &p.Error, &p.Unresolved, &p.StartedAt, &p.UpdatedAt, &p.FinishedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &p, nil
}
