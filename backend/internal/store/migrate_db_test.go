package store

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func appliedMigrations(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT name || '@' || applied_at::text FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return names
}

// schemaShape lists the current schema's columns, constraints and indexes, so two
// databases built by different routes can be compared.
func schemaShape(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' '
		       || is_nullable || ' ' || coalesce(column_default, '')
		  FROM information_schema.columns WHERE table_schema = current_schema()
		UNION ALL
		SELECT 'constraint ' || c.conrelid::regclass::text || ' ' || c.conname || ' '
		       || pg_get_constraintdef(c.oid)
		  FROM pg_constraint c WHERE c.connamespace = current_schema()::regnamespace
		UNION ALL
		SELECT 'index ' || tablename || ' ' || indexname
		  FROM pg_indexes WHERE schemaname = current_schema()
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	shape, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return shape
}

func TestMigrateEmptyDatabaseThenNoop(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ms, err := loadMigrations(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := appliedMigrations(t, pool)
	if len(first) != len(ms) {
		t.Fatalf("recorded %d migrations, want %d: %v", len(first), len(ms), first)
	}
	shape := schemaShape(t, pool)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}
	// Same rows with the same applied_at: nothing re-ran.
	if again := appliedMigrations(t, pool); !reflect.DeepEqual(again, first) {
		t.Fatalf("second run changed schema_migrations:\n%v\nwant\n%v", again, first)
	}
	if again := schemaShape(t, pool); !reflect.DeepEqual(again, shape) {
		t.Fatal("second run changed the schema")
	}
}

func TestMigrateRollsBackFailedFile(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ms := []migration{
		{version: 1, name: "0001_ok", sql: `CREATE TABLE ok_table (x int);`},
		{version: 2, name: "0002_broken", sql: `CREATE TABLE half_table (x int); SELECT no_such_column FROM ok_table;`},
		{version: 3, name: "0003_after", sql: `CREATE TABLE after_table (x int);`},
	}
	if err := migrate(ctx, pool, ms); err == nil {
		t.Fatal("want an error from 0002_broken")
	}

	exists := func(table string) bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !exists("ok_table") {
		t.Error("0001_ok was rolled back; it should stay applied")
	}
	if exists("half_table") {
		t.Error("0002_broken's first statement survived the failure")
	}
	if exists("after_table") {
		t.Error("0003_after ran past a failed migration")
	}
	var versions []int
	rows, _ := pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(versions, []int{1}) {
		t.Fatalf("recorded versions %v, want [1]", versions)
	}
}

// Prod was built by schema.sql, re-applied on every boot, with no schema_migrations
// table. testdata/schema_pre_migrations.sql is that file as of the last release
// without migrations. Upgrading such a database must keep its data and end up the
// same shape as one built from scratch.
func TestMigrateUpgradesPreMigrationDatabase(t *testing.T) {
	ctx := context.Background()
	legacy, err := os.ReadFile("testdata/schema_pre_migrations.sql")
	if err != nil {
		t.Fatal(err)
	}

	old := testPool(t)
	if _, err := old.Exec(ctx, string(legacy)); err != nil {
		t.Fatalf("apply pre-migration schema: %v", err)
	}
	if _, err := old.Exec(ctx, `
		INSERT INTO users (username, display_name, role) VALUES
			('admin', 'Admin', 'admin'), ('owner', 'Owner', 'user'), ('outsider', 'Outsider', 'user');
		INSERT INTO cubes (name, owner_id) SELECT 'private', id FROM users WHERE username = 'owner';
		INSERT INTO cube_members (cube_id, user_id)
			SELECT c.id, u.id FROM cubes c, users u WHERE u.username = 'owner';`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Migrate(ctx, old); err != nil {
		t.Fatalf("migrate pre-migration database: %v", err)
	}

	var users, cubes, members int
	if err := old.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users), (SELECT count(*) FROM cubes), (SELECT count(*) FROM cube_members)`,
	).Scan(&users, &cubes, &members); err != nil {
		t.Fatal(err)
	}
	if users != 3 || cubes != 1 || members != 1 {
		t.Fatalf("after upgrade: %d users, %d cubes, %d members; want 3, 1, 1", users, cubes, members)
	}

	fresh := testPool(t)
	if err := Migrate(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if got, want := schemaShape(t, old), schemaShape(t, fresh); !reflect.DeepEqual(got, want) {
		t.Fatalf("upgraded schema differs from a fresh build:\n%s", diffLines(got, want))
	}
}

func diffLines(got, want []string) string {
	in := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	g, w := in(got), in(want)
	var out string
	for _, x := range got {
		if !w[x] {
			out += "+ " + x + "\n"
		}
	}
	for _, x := range want {
		if !g[x] {
			out += "- " + x + "\n"
		}
	}
	return out
}
