package dbrestore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListAndPath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"mtg_meta_20261101T000000Z_abc1234.dump",
		"mtg_meta_20261115T000000Z_def5678.dump",
		".mtg_meta_20261116T000000Z_0000000.dump.partial", // a dump in flight
		"notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	dumps, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range dumps {
		names = append(names, d.Name)
	}
	want := []string{"mtg_meta_20261115T000000Z_def5678.dump", "mtg_meta_20261101T000000Z_abc1234.dump"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("List = %v, want %v", names, want)
	}

	if p, err := Path(dir, want[0]); err != nil || p != filepath.Join(dir, want[0]) {
		t.Errorf("Path(%q) = %q, %v", want[0], p, err)
	}
	for _, bad := range []string{"../etc/passwd.dump", "/abs.dump", "a/b.dump", ".hidden.dump", "notes.txt", ""} {
		if _, err := Path(dir, bad); !errors.Is(err, ErrBadName) {
			t.Errorf("Path(%q) err = %v, want ErrBadName", bad, err)
		}
	}
	if _, err := Path(dir, "missing.dump"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Path(missing) err = %v, want not-exist", err)
	}
}

func TestStartRejectsBadNameWithoutEnteringMaintenance(t *testing.T) {
	r := NewRunner(nil, t.TempDir(), func() { t.Error("paused") }, func() {})
	if err := r.Start("../x.dump"); !errors.Is(err, ErrBadName) {
		t.Fatalf("err = %v", err)
	}
	if r.InMaintenance() || r.Status().State != StateIdle {
		t.Fatalf("bad name changed state: %+v", r.Status())
	}
}
