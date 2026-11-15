package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// A non-empty dedupKey coalesces with an existing pending job of that key.
func (s *Store) EnqueueJob(ctx context.Context, jobType string, payload any, dedupKey string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var key *string
	if dedupKey != "" {
		key = &dedupKey
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO jobs (type, payload, dedup_key)
		VALUES ($1,$2,$3)
		ON CONFLICT (dedup_key) WHERE status='pending' DO NOTHING`,
		jobType, raw, key)
	return err
}

// Returns (nil, ErrNotFound) when the queue is empty.
func (s *Store) ClaimNextJob(ctx context.Context) (*domain.Job, error) {
	var j domain.Job
	err := s.pool.QueryRow(ctx, `
		UPDATE jobs SET status='running', started_at=now(), attempts=attempts+1
		WHERE id = (
			SELECT id FROM jobs
			WHERE status='pending' AND scheduled_at <= now()
			ORDER BY scheduled_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, type, payload, status, attempts`,
	).Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Attempts)
	if err != nil {
		return nil, normErr(err)
	}
	return &j, nil
}

func (s *Store) CompleteJob(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status='done', finished_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) FailJob(ctx context.Context, id uuid.UUID, msg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status='failed', finished_at=now(), last_error=$2 WHERE id=$1`, id, msg)
	return err
}

// RetryJob puts a failed attempt back in the queue to run after delay. When a
// newer pending job holds the same dedup_key, that one already covers the work and
// re-queueing would break the one-pending-per-key index, so this one fails instead.
func (s *Store) RetryJob(ctx context.Context, id uuid.UUID, msg string, delay time.Duration) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE jobs j SET status='pending', started_at=NULL, last_error=$2,
			scheduled_at=now() + make_interval(secs => $3)
		WHERE j.id=$1 AND NOT EXISTS (
			SELECT 1 FROM jobs p
			WHERE p.status='pending' AND p.dedup_key=j.dedup_key)`,
		id, msg, delay.Seconds())
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return s.FailJob(ctx, id, msg+" (not retried: superseded by a pending job)")
	}
	return nil
}

// RecoverJobs runs at boot, before the worker starts, and returns jobs a restart
// left 'running' to the queue; nothing else would ever pick them up again. A job
// already on its last attempt fails instead, so one that kills the process cannot
// crash-loop it, as does one a newer pending job with its dedup_key supersedes.
func (s *Store) RecoverJobs(ctx context.Context, maxAttempts int) (requeued, failed int64, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ct, err := tx.Exec(ctx, `
		UPDATE jobs j SET status='pending', started_at=NULL, scheduled_at=now()
		WHERE j.status='running' AND j.attempts < $1 AND NOT EXISTS (
			SELECT 1 FROM jobs p
			WHERE p.status='pending' AND p.dedup_key=j.dedup_key)`, maxAttempts)
	if err != nil {
		return 0, 0, err
	}
	requeued = ct.RowsAffected()
	ct, err = tx.Exec(ctx, `
		UPDATE jobs SET status='failed', finished_at=now(),
			last_error='interrupted by a restart; not retried'
		WHERE status='running'`)
	if err != nil {
		return 0, 0, err
	}
	failed = ct.RowsAffected()
	return requeued, failed, tx.Commit(ctx)
}

// PruneJobs deletes finished jobs older than before. Pending and running rows are
// live queue state and never pruned.
func (s *Store) PruneJobs(ctx context.Context, before time.Time) (int64, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM jobs WHERE status IN ('done','failed') AND finished_at < $1`, before)
	return ct.RowsAffected(), err
}

// PruneAnalyticsRuns deletes non-current runs started before before; their stat
// rows (card_pair_stats included) go with them by ON DELETE CASCADE. Nothing
// reads a superseded run, and a run still 'running' that old died with a restart.
func (s *Store) PruneAnalyticsRuns(ctx context.Context, before time.Time) (int64, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM analytics_runs WHERE NOT is_current AND started_at < $1`, before)
	return ct.RowsAffected(), err
}
