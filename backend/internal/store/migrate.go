package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema changes are numbered files in migrations/, each applied exactly once, in
// version order, in its own transaction. An applied file is never edited: change the
// schema by adding the next number. Because a file runs once, it may hold statements
// that are not idempotent (backfills, DROP COLUMN) — the old re-run-schema.sql-on-
// every-boot model could not, and a backfill re-running forever is how a private cube
// once got opened to every user.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string // file name without the .sql suffix, e.g. "0001_baseline"
	sql     string
}

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// loadMigrations reads every .sql file in dir, sorted by version. A misnamed file or
// a reused version number is an error rather than a skip: a typo would otherwise drop
// a migration silently.
func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var ms []migration
	seen := map[int]string{}
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migration %q: name must be NNNN_description.sql", e.Name())
		}
		version, _ := strconv.Atoi(m[1])
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", prev, e.Name(), version)
		}
		seen[version] = e.Name()
		body, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: version, name: e.Name()[:len(e.Name())-4], sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

// migrationDB is the slice of the database the runner needs, so ordering and
// skipping can be unit-tested without Postgres.
type migrationDB interface {
	appliedVersions(ctx context.Context) (map[int]bool, error)
	// apply runs m and records it as applied, atomically.
	apply(ctx context.Context, m migration) error
}

// runMigrations applies, in order, every migration not yet recorded, and stops at the
// first failure. It returns the names it applied.
func runMigrations(ctx context.Context, db migrationDB, ms []migration) ([]string, error) {
	done, err := db.appliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	var applied []string
	for _, m := range ms {
		if done[m.version] {
			continue
		}
		if err := db.apply(ctx, m); err != nil {
			return applied, fmt.Errorf("migration %s: %w", m.name, err)
		}
		applied = append(applied, m.name)
	}
	return applied, nil
}

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    integer PRIMARY KEY,
    name       text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// An arbitrary constant shared by every backend process, so two booting at once (a
// deploy overlapping a restart) cannot both apply the same file.
const migrationLockKey int64 = 7_404_202_611

// migrationConn is what the runner executes on: a connection at boot, or the
// transaction a restore rebuilds the schema in (see Reimport).
type migrationConn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

type pgMigrationDB struct{ conn migrationConn }

func (p pgMigrationDB) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := p.conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, err
	}
	done := make(map[int]bool, len(versions))
	for _, v := range versions {
		done[v] = true
	}
	return done, nil
}

func (p pgMigrationDB) apply(ctx context.Context, m migration) error {
	return pgx.BeginFunc(ctx, p.conn, func(tx pgx.Tx) error {
		// No arguments, so pgx sends the multi-statement file over the simple protocol.
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name)
		return err
	})
}

// Migrate brings the database up to the latest schema: on an empty database it builds
// the whole schema, on a current one it does nothing. The backend calls it at boot,
// before it listens; it is also the entry point for rebuilding a schema to restore a
// dump into.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	ms, err := loadMigrations(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	return migrate(ctx, pool, ms)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, ms []migration) error {
	// The advisory lock is per-session, so every step runs on this one connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey)

	if _, err := conn.Exec(ctx, createMigrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, err := runMigrations(ctx, pgMigrationDB{conn.Conn()}, ms)
	for _, name := range applied {
		log.Printf("applied migration %s", name)
	}
	return err
}

// Migrate applies pending migrations to the store's database. See Migrate.
func (s *Store) Migrate(ctx context.Context) error { return Migrate(ctx, s.pool) }
