package store

import (
	"context"
	_ "embed"
)

//go:embed schema.sql
var schemaSQL string

// EnsureSchema applies the idempotent schema (CREATE ... IF NOT EXISTS) so a fresh or
// partially-initialized database converges to the expected shape without touching
// existing data. It runs on every startup; pgx executes the multi-statement script via
// the simple protocol because there are no query arguments.
func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}
