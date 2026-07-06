package store

import (
	"context"
	"encoding/json"

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
