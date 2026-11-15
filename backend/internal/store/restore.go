package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RestoreDump flashes dst from a pg_dump -Fc file, whatever schema version wrote it:
// the dump is restored as-is into a scratch database on the same server, then
// Reimport rebuilds dst at the latest schema and copies the data across. The scratch
// database is dropped afterwards, success or not. Needs pg_restore on PATH and a
// role allowed to CREATE DATABASE.
func RestoreDump(ctx context.Context, dst *pgxpool.Pool, file string) (*ReimportReport, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, err
	}
	scratch := "restore_scratch_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ident := pgx.Identifier{scratch}.Sanitize()
	// template0, not template1: anything installed into template1 would collide
	// with the dump's own CREATE statements and trip --exit-on-error.
	if _, err := dst.Exec(ctx, "CREATE DATABASE "+ident+" TEMPLATE template0"); err != nil {
		return nil, fmt.Errorf("create scratch database: %w", err)
	}
	defer func() {
		if _, err := dst.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident); err != nil {
			fmt.Fprintf(os.Stderr, "restore: drop scratch database %s: %v\n", scratch, err)
		}
	}()

	cfg := dst.Config().Copy()
	cfg.ConnConfig.Database = scratch
	cmd := exec.CommandContext(ctx, "pg_restore",
		"--no-owner", "--no-privileges", "--exit-on-error", "--dbname", scratch, file)
	cmd.Env = append(os.Environ(), libpqEnv(cfg)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pg_restore: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	src, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Closed before the deferred DROP DATABASE, which fails while it has sessions.
	defer src.Close()
	schema, err := dumpSchema(ctx, src)
	if err != nil {
		return nil, err
	}
	return Reimport(ctx, src, schema, dst)
}

// libpqEnv points pg_restore at the same server and role as the pool. Environment
// rather than a URL argument keeps the password out of the process list.
func libpqEnv(cfg *pgxpool.Config) []string {
	cc := cfg.ConnConfig
	// pgx expands sslmode=prefer into TLS with a plaintext fallback.
	ssl := "require"
	if cc.TLSConfig == nil {
		ssl = "disable"
	} else {
		for _, fb := range cc.Fallbacks {
			if fb.TLSConfig == nil {
				ssl = "prefer"
			}
		}
	}
	return []string{
		"PGHOST=" + cc.Host,
		"PGPORT=" + strconv.Itoa(int(cc.Port)),
		"PGUSER=" + cc.User,
		"PGPASSWORD=" + cc.Password,
		"PGSSLMODE=" + ssl,
	}
}

// dumpSchema finds the schema the dump's tables landed in. Prod dumps are of
// public, but nothing here depends on that.
func dumpSchema(ctx context.Context, src *pgxpool.Pool) (string, error) {
	rows, err := src.Query(ctx, `
		SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relname = 'users' AND c.relkind = 'r'`)
	if err != nil {
		return "", err
	}
	schemas, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	if len(schemas) != 1 {
		return "", fmt.Errorf("dump has %d schemas with a users table, want 1: %v", len(schemas), schemas)
	}
	return schemas[0], nil
}
