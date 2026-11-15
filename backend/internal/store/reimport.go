package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TableReport is what a reimport did to one table.
type TableReport struct {
	Table   string `json:"table"`
	Copied  int    `json:"copied"`
	Skipped int    `json:"skipped"`
	// DroppedColumns are in the dump but not the schema: their data is discarded.
	// A renamed column shows up here and in BlankedColumns.
	DroppedColumns []string `json:"dropped_columns,omitempty"`
	// BlankedColumns are in the schema but not the dump: they get their default, or NULL.
	BlankedColumns []string `json:"blanked_columns,omitempty"`
	// SkipReasons holds the first few rejected rows' errors, keyed by the row's
	// first column (normally its id).
	SkipReasons []string `json:"skip_reasons,omitempty"`
	// Note flags a table that exists on only one side.
	Note string `json:"note,omitempty"`
}

type ReimportReport struct {
	// Upgraded lists the migrations applied to the dump before it was copied.
	Upgraded []string `json:"upgraded,omitempty"`
	// UpgradeError is set when one of those migrations failed: the copy went ahead
	// from the last version that applied, so data that migration would have moved
	// between columns may be missing.
	UpgradeError string        `json:"upgrade_error,omitempty"`
	Tables       []TableReport `json:"tables"`
}

const maxSkipReasons = 20

// WriteText prints the report as a table, one indented line per detail.
func (r *ReimportReport) WriteText(w io.Writer) {
	if len(r.Upgraded) > 0 {
		fmt.Fprintf(w, "upgraded the dump with: %s\n", strings.Join(r.Upgraded, ", "))
	}
	if r.UpgradeError != "" {
		fmt.Fprintf(w, "WARNING: could not upgrade the dump, copied it as it was: %s\n", r.UpgradeError)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TABLE\tCOPIED\tSKIPPED")
	for _, t := range r.Tables {
		fmt.Fprintf(tw, "%s\t%d\t%d\n", t.Table, t.Copied, t.Skipped)
		if t.Note != "" {
			fmt.Fprintf(tw, "  %s\t\t\n", t.Note)
		}
		if len(t.DroppedColumns) > 0 {
			fmt.Fprintf(tw, "  dropped columns: %s\t\t\n", strings.Join(t.DroppedColumns, ", "))
		}
		if len(t.BlankedColumns) > 0 {
			fmt.Fprintf(tw, "  blanked columns: %s\t\t\n", strings.Join(t.BlankedColumns, ", "))
		}
		for _, s := range t.SkipReasons {
			fmt.Fprintf(tw, "  skipped %s\t\t\n", s)
		}
	}
	tw.Flush()
}

// Reimport replaces the dst pool's current schema with the latest one built from the
// migrations, then copies srcSchema's data from src into it, table by table, best
// effort. It first migrates src itself to the latest schema (see upgradeSource), so
// src must be a disposable copy — RestoreDump's scratch database. It is the restore path's core, and only the restore path's: a deploy
// migrates in place, because this discards every dropped or renamed column.
//
// The drop, the rebuild and the copy share one transaction, so a failure part-way
// leaves dst exactly as it was. Rows the new schema rejects are skipped and
// reported, not fatal.
func Reimport(ctx context.Context, src *pgxpool.Pool, srcSchema string, dst *pgxpool.Pool) (*ReimportReport, error) {
	ms, err := loadMigrations(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	srcConn, err := src.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer srcConn.Release()
	upgraded, upgradeErr, err := upgradeSource(ctx, srcConn.Conn(), srcSchema, ms)
	if err != nil {
		return nil, fmt.Errorf("upgrade dump: %w", err)
	}
	srcCols, err := tableColumns(ctx, srcConn.Conn(), srcSchema)
	if err != nil {
		return nil, fmt.Errorf("read dump tables: %w", err)
	}

	var report *ReimportReport
	err = pgx.BeginFunc(ctx, dst, func(tx pgx.Tx) error {
		// Holds off a backend booting mid-restore until the new schema is committed.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockKey); err != nil {
			return fmt.Errorf("lock migrations: %w", err)
		}
		var schema string
		if err := tx.QueryRow(ctx, `SELECT coalesce(current_schema(), '')`).Scan(&schema); err != nil {
			return err
		}
		if schema == "" {
			return fmt.Errorf("target has no current schema")
		}
		ident := pgx.Identifier{schema}.Sanitize()
		if _, err := tx.Exec(ctx, "DROP SCHEMA "+ident+" CASCADE; CREATE SCHEMA "+ident); err != nil {
			return fmt.Errorf("recreate schema %s: %w", schema, err)
		}
		if _, err := tx.Exec(ctx, createMigrationsTable); err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}
		if _, err := runMigrations(ctx, pgMigrationDB{tx}, ms); err != nil {
			return err
		}
		report, err = copyTables(ctx, srcConn.Conn(), srcSchema, srcCols, tx, schema)
		return err
	})
	if err != nil {
		return nil, err
	}
	report.Upgraded = upgraded
	if upgradeErr != nil {
		report.UpgradeError = upgradeErr.Error()
	}
	return report, nil
}

// upgradeSource applies the pending migrations to the dump's own schema, before
// anything is copied. The copy matches columns by name, so data a migration moves
// between columns — 0002 turns decklists.cube_id into a cube tag — would otherwise be
// dropped on one side and blanked on the other. A dump from before versioned
// migrations has no schema_migrations and takes the idempotent baseline first.
//
// Each file commits on its own, so a failing one (upgradeErr) leaves src at the last
// version that applied, and the copy goes ahead from there best-effort, as it did
// before this step existed. err is for failures that leave nothing to copy from.
func upgradeSource(ctx context.Context, conn *pgx.Conn, schema string, ms []migration) (applied []string, upgradeErr, err error) {
	// The schema alone, not "schema, public": an unqualified name a dump lacks must
	// fail, not land on whatever public holds in the same database.
	if _, err := conn.Exec(ctx, "SET search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return nil, nil, err
	}
	defer conn.Exec(context.Background(), "RESET search_path")
	if _, err := conn.Exec(ctx, createMigrationsTable); err != nil {
		return nil, nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, upgradeErr = runMigrations(ctx, pgMigrationDB{conn}, ms)
	return applied, upgradeErr, nil
}

type column struct {
	name      string
	generated bool
}

func tableColumns(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, schema string) (map[string][]column, error) {
	rows, err := q.Query(ctx, `
		SELECT c.relname, a.attname, a.attgenerated <> ''
		  FROM pg_attribute a
		  JOIN pg_class c ON c.oid = a.attrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relkind = 'r' AND a.attnum > 0 AND NOT a.attisdropped
		 ORDER BY c.relname, a.attnum`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := map[string][]column{}
	for rows.Next() {
		var table string
		var col column
		if err := rows.Scan(&table, &col.name, &col.generated); err != nil {
			return nil, err
		}
		tables[table] = append(tables[table], col)
	}
	return tables, rows.Err()
}

// fkOrder sorts tables so every table comes after the tables its foreign keys
// reference. Self-references are ignored; a cycle, which this schema does not have,
// falls back to name order for the tables in it.
func fkOrder(ctx context.Context, tx pgx.Tx, schema string, tables []string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.relname, p.relname
		  FROM pg_constraint k
		  JOIN pg_class c ON c.oid = k.conrelid
		  JOIN pg_class p ON p.oid = k.confrelid
		 WHERE k.contype = 'f' AND k.connamespace = $1::regnamespace AND c.oid <> p.oid`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deps := map[string]map[string]bool{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			return nil, err
		}
		if deps[child] == nil {
			deps[child] = map[string]bool{}
		}
		deps[child][parent] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	remaining := append([]string(nil), tables...)
	sort.Strings(remaining)
	done := map[string]bool{}
	var order []string
	for len(remaining) > 0 {
		var next []string
		for _, t := range remaining {
			ready := true
			for p := range deps[t] {
				if !done[p] {
					ready = false
					break
				}
			}
			if ready {
				order = append(order, t)
			} else {
				next = append(next, t)
			}
		}
		if len(next) == len(remaining) { // a cycle
			order = append(order, next...)
			break
		}
		for _, t := range order {
			done[t] = true
		}
		remaining = next
	}
	return order, nil
}

func copyTables(ctx context.Context, src *pgx.Conn, srcSchema string, srcCols map[string][]column,
	tx pgx.Tx, schema string) (*ReimportReport, error) {
	dstCols, err := tableColumns(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	var names []string
	for t := range dstCols {
		if t != "schema_migrations" {
			names = append(names, t)
		}
	}
	order, err := fkOrder(ctx, tx, schema, names)
	if err != nil {
		return nil, err
	}

	report := &ReimportReport{}
	for _, table := range order {
		tr := TableReport{Table: table}
		from, ok := srcCols[table]
		if !ok {
			tr.Note = "not in the dump; left empty"
			report.Tables = append(report.Tables, tr)
			continue
		}
		inSrc := map[string]bool{}
		for _, c := range from {
			inSrc[c.name] = true
		}
		inDst := map[string]bool{}
		var cols []string
		for _, c := range dstCols[table] {
			inDst[c.name] = true
			switch {
			case c.generated: // recomputed from the copied columns
			case inSrc[c.name]:
				cols = append(cols, pgx.Identifier{c.name}.Sanitize())
			default:
				tr.BlankedColumns = append(tr.BlankedColumns, c.name)
			}
		}
		for _, c := range from {
			if !inDst[c.name] {
				tr.DroppedColumns = append(tr.DroppedColumns, c.name)
			}
		}
		if len(cols) > 0 {
			if err := copyTable(ctx, src, pgx.Identifier{srcSchema, table}.Sanitize(),
				tx, pgx.Identifier{schema, table}.Sanitize(), strings.Join(cols, ", "), &tr); err != nil {
				return nil, fmt.Errorf("copy %s: %w", table, err)
			}
		}
		report.Tables = append(report.Tables, tr)
	}

	for table := range srcCols {
		if _, ok := dstCols[table]; !ok && table != "schema_migrations" {
			report.Tables = append(report.Tables, TableReport{Table: table, Note: "not in the schema; dropped"})
		}
	}
	return report, nil
}

// copyTable moves rows as COPY text, which leaves type conversion to the server, so
// a column whose type widened still copies. It tries the whole table at once and
// only on a failure falls back to one row at a time, each under its own savepoint,
// to find and skip the rows the new schema rejects.
func copyTable(ctx context.Context, src *pgx.Conn, from string, tx pgx.Tx, to, cols string, tr *TableReport) error {
	var buf bytes.Buffer
	if _, err := src.PgConn().CopyTo(ctx, &buf,
		"COPY (SELECT "+cols+" FROM "+from+") TO STDOUT"); err != nil {
		return err
	}
	if buf.Len() == 0 {
		return nil
	}
	copyIn := "COPY " + to + " (" + cols + ") FROM STDIN"

	all := buf.Bytes()
	if ok, err := copyUnderSavepoint(ctx, tx, copyIn, all); err != nil {
		return err
	} else if ok == nil {
		tr.Copied = bytes.Count(all, []byte{'\n'})
		return nil
	}

	// COPY text escapes newlines inside values, so a raw newline always ends a row.
	for _, line := range bytes.SplitAfter(all, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		rowErr, err := copyUnderSavepoint(ctx, tx, copyIn, line)
		if err != nil {
			return err
		}
		if rowErr == nil {
			tr.Copied++
			continue
		}
		tr.Skipped++
		if len(tr.SkipReasons) < maxSkipReasons {
			key, _, _ := bytes.Cut(line, []byte{'\t'})
			tr.SkipReasons = append(tr.SkipReasons,
				fmt.Sprintf("%s: %v", bytes.TrimRight(key, "\n"), rowErr))
		}
	}
	return nil
}

// copyUnderSavepoint returns the COPY's own error as rowErr, rolled back to the
// savepoint, and reserves err for failures that leave the transaction unusable.
func copyUnderSavepoint(ctx context.Context, tx pgx.Tx, copyIn string, data []byte) (rowErr, err error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, rowErr = sp.Conn().PgConn().CopyFrom(ctx, bytes.NewReader(data), copyIn); rowErr != nil {
		return rowErr, sp.Rollback(ctx)
	}
	return nil, sp.Commit(ctx)
}
