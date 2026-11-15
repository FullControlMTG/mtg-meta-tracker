package httpapi

import (
	"errors"
	"net/http"
	"os"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/dbrestore"
)

// restoreConfirmation must be sent verbatim: a restore replaces every table, and
// a stray POST should not be able to start one.
const restoreConfirmation = "replace the database"

// maintenance answers 503 to everything but the health check while a restore
// runs. Health stays up so the container is not marked unhealthy mid-restore.
func (s *Server) maintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.restore != nil && s.restore.InMaintenance() && r.URL.Path != "/api/health" {
			w.Header().Set("Retry-After", "30")
			writeErr(w, http.StatusServiceUnavailable, "database restore in progress")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleListDumps(w http.ResponseWriter, r *http.Request) {
	dumps, err := dbrestore.List(s.restore.Dir())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "cannot read dump directory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dumps": dumps})
}

// handleRestoreStatus reports the last restore this process ran. A restore puts
// the API in maintenance, so it is only readable once that one has finished.
func (s *Server) handleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.restore.Status())
}

func (s *Server) handleStartRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dump    string `json:"dump"`
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Confirm != restoreConfirmation {
		writeErr(w, http.StatusBadRequest, `confirm must be "`+restoreConfirmation+`"`)
		return
	}
	switch err := s.restore.Start(req.Dump); {
	case err == nil:
		writeJSON(w, http.StatusAccepted, s.restore.Status())
	case errors.Is(err, dbrestore.ErrBadName):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, os.ErrNotExist):
		writeErr(w, http.StatusNotFound, "no such dump")
	case errors.Is(err, dbrestore.ErrBusy):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "cannot read dump")
	}
}
