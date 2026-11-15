package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/dbrestore"
)

func TestStartRestoreRequiresConfirmation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.dump"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{restore: dbrestore.NewRunner(nil, dir, func() { t.Error("paused") }, func() {})}

	for body, want := range map[string]int{
		`{"dump":"a.dump"}`:                                     http.StatusBadRequest,
		`{"dump":"a.dump","confirm":"yes"}`:                     http.StatusBadRequest,
		`{"dump":"../a.dump","confirm":"replace the database"}`: http.StatusBadRequest,
		`{"dump":"b.dump","confirm":"replace the database"}`:    http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		s.handleStartRestore(rec, httptest.NewRequest(http.MethodPost, "/api/admin/db/restore", strings.NewReader(body)))
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d (%s)", body, rec.Code, want, rec.Body)
		}
	}
	if s.restore.InMaintenance() {
		t.Fatal("a rejected request entered maintenance")
	}
}

func TestMaintenanceOffPassesThrough(t *testing.T) {
	s := &Server{restore: dbrestore.NewRunner(nil, t.TempDir(), func() {}, func() {})}
	h := s.maintenance(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/cubes", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status %d", rec.Code)
	}
}
