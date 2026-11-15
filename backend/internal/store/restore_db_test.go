package store

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// legacySource builds a database as an older release left it: the pre-migration
// schema, drifted from the latest in each way a reimport has to absorb, with rows.
// It returns the pool and the schema the rows live in.
func legacySource(t *testing.T) (*Store, string) {
	t.Helper()
	ctx := context.Background()
	legacy, err := os.ReadFile("testdata/schema_pre_migrations.sql")
	if err != nil {
		t.Fatal(err)
	}
	src := New(testPool(t))
	if _, err := src.pool.Exec(ctx, string(legacy)); err != nil {
		t.Fatalf("apply pre-migration schema: %v", err)
	}
	if _, err := src.pool.Exec(ctx, `
		-- A column the latest schema no longer has, one it has gained, and a CHECK
		-- it has tightened.
		ALTER TABLE users ADD COLUMN favorite_color text;
		ALTER TABLE cubes DROP COLUMN description;
		ALTER TABLE users DROP CONSTRAINT users_role_check;

		INSERT INTO users (username, display_name, role, favorite_color) VALUES
			('admin', 'Admin', 'admin', 'blue'), ('owner', 'Owner', 'user', 'red'),
			('mod', 'Mod', 'moderator', 'green');
		INSERT INTO cubes (name, owner_id) SELECT 'vintage', id FROM users WHERE username = 'owner';
		INSERT INTO cube_members (cube_id, user_id) SELECT c.id, u.id FROM cubes c, users u;
		INSERT INTO cards (scryfall_id, name) VALUES
			('11111111-1111-1111-1111-111111111111', 'Jace, the Mind Sculptor');
		-- A deck linked to its cube by the cube_id column that 0002 replaces with a tag.
		INSERT INTO decklists (cube_id, user_id, name, decklist_raw)
			SELECT c.id, u.id, 'Burn', '4 Lightning Bolt' FROM cubes c, users u WHERE u.username = 'owner';`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var schema string
	if err := src.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	return src, schema
}

func checkReimported(t *testing.T, dst *Store, report *ReimportReport) {
	t.Helper()
	ctx := context.Background()
	byTable := map[string]TableReport{}
	for _, tr := range report.Tables {
		byTable[tr.Table] = tr
	}

	users := byTable["users"]
	if users.Copied != 2 || users.Skipped != 1 {
		t.Errorf("users: copied %d, skipped %d; want 2, 1", users.Copied, users.Skipped)
	}
	if !reflect.DeepEqual(users.DroppedColumns, []string{"favorite_color"}) {
		t.Errorf("users dropped columns = %v, want [favorite_color]", users.DroppedColumns)
	}
	if len(users.SkipReasons) != 1 || !strings.Contains(users.SkipReasons[0], "users_role_check") {
		t.Errorf("users skip reasons = %v, want the role check", users.SkipReasons)
	}
	if cubes := byTable["cubes"]; !reflect.DeepEqual(cubes.BlankedColumns, []string{"description"}) {
		t.Errorf("cubes blanked columns = %v, want [description]", cubes.BlankedColumns)
	}
	// The skipped user's membership fails its foreign key in turn.
	if m := byTable["cube_members"]; m.Copied != 2 || m.Skipped != 1 {
		t.Errorf("cube_members: copied %d, skipped %d; want 2, 1", m.Copied, m.Skipped)
	}
	if _, ok := byTable["schema_migrations"]; ok {
		t.Error("schema_migrations was copied")
	}
	if report.UpgradeError != "" || len(report.Upgraded) == 0 {
		t.Errorf("dump upgrade: applied %v, error %q; want every migration, no error",
			report.Upgraded, report.UpgradeError)
	}
	// The dump is migrated before the copy, so the cube link moves into the tags
	// instead of being dropped as a column the schema no longer has.
	if dl := byTable["decklists"]; dl.Copied != 1 || len(dl.DroppedColumns) != 0 {
		t.Errorf("decklists: copied %d, dropped %v; want 1 row, no columns", dl.Copied, dl.DroppedColumns)
	}
	var inCube bool
	if err := dst.pool.QueryRow(ctx, `
		SELECT d.tags = ARRAY[c.deck_tag] AND d.format = 'cube' FROM decklists d, cubes c`,
	).Scan(&inCube); err != nil {
		t.Fatal(err)
	}
	if !inCube {
		t.Error("reimported deck is not filed under its cube")
	}

	var usernames []string
	rows, _ := dst.pool.Query(ctx, `SELECT username FROM users ORDER BY username`)
	for rows.Next() {
		var u string
		_ = rows.Scan(&u)
		usernames = append(usernames, u)
	}
	if !reflect.DeepEqual(usernames, []string{"admin", "owner"}) {
		t.Errorf("users after reimport = %v, want [admin owner]", usernames)
	}
	var description *string
	var slug string
	if err := dst.pool.QueryRow(ctx, `SELECT description FROM cubes`).Scan(&description); err != nil {
		t.Fatal(err)
	}
	if description != nil {
		t.Errorf("blanked cubes.description = %q, want NULL", *description)
	}
	if err := dst.pool.QueryRow(ctx, `SELECT slug FROM cards`).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "jace-the-mind-sculptor" {
		t.Errorf("generated slug = %q", slug)
	}

	fresh := testPool(t)
	if err := Migrate(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if got, want := schemaShape(t, dst.pool), schemaShape(t, fresh); !reflect.DeepEqual(got, want) {
		t.Fatalf("reimported schema differs from a fresh build:\n%s", diffLines(got, want))
	}
}

func TestReimportIntoLatestSchema(t *testing.T) {
	ctx := context.Background()
	src, schema := legacySource(t)

	// The target starts with data of its own, which the reimport must replace.
	dst := testStore(t)
	mustUser(t, dst, "stale", "user")

	report, err := Reimport(ctx, src.pool, schema, dst.pool)
	if err != nil {
		t.Fatalf("reimport: %v", err)
	}
	checkReimported(t, dst, report)
}

// The end-to-end path: pg_dump a pre-migration database and flash it.
func TestRestoreDumpFromPreMigrationSchema(t *testing.T) {
	for _, bin := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	ctx := context.Background()
	src, schema := legacySource(t)

	file := filepath.Join(t.TempDir(), "legacy.dump")
	cmd := exec.Command("pg_dump", "-Fc", "--schema", schema, "-f", file, src.pool.Config().ConnConfig.Database)
	cmd.Env = append(os.Environ(), libpqEnv(src.pool.Config())...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v: %s", err, out)
	}

	dst := testStore(t)
	report, err := RestoreDump(ctx, dst.pool, file)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	checkReimported(t, dst, report)

	var scratch int
	if err := dst.pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_database WHERE datname LIKE 'restore_scratch_%'`).Scan(&scratch); err != nil {
		t.Fatal(err)
	}
	if scratch != 0 {
		t.Errorf("%d scratch databases left behind", scratch)
	}
}
