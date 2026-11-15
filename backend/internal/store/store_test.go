package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormErr(t *testing.T) {
	if err := normErr(fmt.Errorf("scan: %w", pgx.ErrNoRows)); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrapped ErrNoRows -> %v, want ErrNotFound", err)
	}
	other := errors.New("connection reset")
	if err := normErr(other); err != other {
		t.Errorf("other error -> %v, want it unchanged", err)
	}
	if normErr(nil) != nil {
		t.Error("nil -> non-nil")
	}
}

func TestFrontFace(t *testing.T) {
	for in, want := range map[string]string{
		"Fire // Ice":                   "Fire",
		"Delver of Secrets / Insectile": "Delver of Secrets",
		"Lightning Bolt":                "Lightning Bolt",
		"":                              "",
	} {
		if got := FrontFace(in); got != want {
			t.Errorf("FrontFace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeref(t *testing.T) {
	s := "x"
	if deref(nil) != "" || deref(&s) != "x" {
		t.Fatal("deref")
	}
}

func TestComboWriteErr(t *testing.T) {
	dup := &pgconn.PgError{Code: "23505"}
	if err := comboWriteErr(fmt.Errorf("insert: %w", dup)); !errors.Is(err, ErrComboNameTaken) {
		t.Errorf("unique violation -> %v, want ErrComboNameTaken", err)
	}
	fk := &pgconn.PgError{Code: "23503"}
	if err := comboWriteErr(fk); err != error(fk) {
		t.Errorf("FK violation -> %v, want it unchanged", err)
	}
}
