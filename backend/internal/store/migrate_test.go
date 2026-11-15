package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// fakeMigrationDB records what ran. A failing version neither runs nor is recorded,
// mirroring the transaction rollback in pgMigrationDB.apply.
type fakeMigrationDB struct {
	done    map[int]bool
	ran     []string
	failsAt int
}

func (f *fakeMigrationDB) appliedVersions(context.Context) (map[int]bool, error) {
	out := map[int]bool{}
	for v := range f.done {
		out[v] = true
	}
	return out, nil
}

func (f *fakeMigrationDB) apply(_ context.Context, m migration) error {
	if m.version == f.failsAt {
		return errors.New("syntax error")
	}
	f.done[m.version] = true
	f.ran = append(f.ran, m.name)
	return nil
}

func TestLoadMigrationsOrdersByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0010_ten.sql": {Data: []byte("SELECT 10;")},
		"m/0002_two.sql": {Data: []byte("SELECT 2;")},
		"m/0001_one.sql": {Data: []byte("SELECT 1;")},
	}
	ms, err := loadMigrations(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.name)
	}
	want := []string{"0001_one", "0002_two", "0010_ten"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if ms[2].version != 10 || ms[2].sql != "SELECT 10;" {
		t.Fatalf("last migration = %+v", ms[2])
	}
}

func TestLoadMigrationsRejectsBadNames(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"misnamed": {"m/1_one.sql": {}},
		"duplicate": {
			"m/0001_one.sql": {},
			"m/0001_uno.sql": {},
		},
	} {
		if _, err := loadMigrations(fsys, "m"); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestEmbeddedMigrationsLoad(t *testing.T) {
	ms, err := loadMigrations(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 || ms[0].name != "0001_baseline" {
		t.Fatalf("first migration = %v, want 0001_baseline", ms)
	}
}

func testMigrations() []migration {
	return []migration{
		{version: 1, name: "0001_a"},
		{version: 2, name: "0002_b"},
		{version: 3, name: "0003_c"},
	}
}

func TestRunMigrationsAppliesInOrder(t *testing.T) {
	db := &fakeMigrationDB{done: map[int]bool{}}
	applied, err := runMigrations(context.Background(), db, testMigrations())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0001_a", "0002_b", "0003_c"}
	if !reflect.DeepEqual(db.ran, want) || !reflect.DeepEqual(applied, want) {
		t.Fatalf("ran %v, returned %v, want %v", db.ran, applied, want)
	}
}

func TestRunMigrationsSkipsApplied(t *testing.T) {
	db := &fakeMigrationDB{done: map[int]bool{1: true, 2: true}}
	if _, err := runMigrations(context.Background(), db, testMigrations()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(db.ran, []string{"0003_c"}) {
		t.Fatalf("ran %v, want only 0003_c", db.ran)
	}

	// A second boot finds everything applied.
	db.ran = nil
	if _, err := runMigrations(context.Background(), db, testMigrations()); err != nil {
		t.Fatal(err)
	}
	if len(db.ran) != 0 {
		t.Fatalf("second run applied %v, want nothing", db.ran)
	}
}

func TestRunMigrationsStopsAtFailure(t *testing.T) {
	db := &fakeMigrationDB{done: map[int]bool{}, failsAt: 2}
	applied, err := runMigrations(context.Background(), db, testMigrations())
	if err == nil || !strings.Contains(err.Error(), "0002_b") {
		t.Fatalf("err = %v, want it to name 0002_b", err)
	}
	// 0003 must not run past a failed 0002: later files may depend on it.
	if !reflect.DeepEqual(db.ran, []string{"0001_a"}) || !reflect.DeepEqual(applied, db.ran) {
		t.Fatalf("ran %v, returned %v, want only 0001_a", db.ran, applied)
	}
	if db.done[2] {
		t.Fatal("failed migration recorded as applied")
	}
}
