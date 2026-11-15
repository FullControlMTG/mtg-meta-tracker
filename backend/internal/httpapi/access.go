package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// Cube membership is the read boundary and cube ownership the write boundary; a site
// admin bypasses both. These helpers write the denial response and return false, so a
// handler reads `if !s.requireCubeAccess(...) { return }`.

type memberStore interface {
	IsCubeMember(ctx context.Context, cubeID, userID uuid.UUID) (bool, error)
	IsCubeOwner(ctx context.Context, cubeID, userID uuid.UUID) (bool, error)
}

// requireCubeAccess gates a cube-scoped read. A non-member gets 404 — they aren't told
// the cube exists.
func (s *Server) requireCubeAccess(w http.ResponseWriter, r *http.Request, cubeID uuid.UUID) bool {
	caller := appctx.From(r.Context())
	if caller.IsAdmin() {
		return true
	}
	member, err := s.members.IsCubeMember(r.Context(), cubeID, caller.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check cube access")
		return false
	}
	if !member {
		writeErr(w, http.StatusNotFound, "cube not found")
		return false
	}
	return true
}

// requireDeckAccess gates reading a deck. A deck filed under a cube is read through
// that cube's membership; one filed under none (its cube was deleted) is visible only
// to its owner and admins, with the same 404 for everyone else.
func (s *Server) requireDeckAccess(w http.ResponseWriter, r *http.Request, d *domain.Decklist) bool {
	if d.CubeID != nil {
		return s.requireCubeAccess(w, r, *d.CubeID)
	}
	if appctx.From(r.Context()).CanMutateOwned(d.UserID) {
		return true
	}
	writeErr(w, http.StatusNotFound, "decklist not found")
	return false
}

// requireCubeOwner gates cube administration to the owner or an admin. A member who
// isn't the owner gets 403; a non-member 404, as with reads.
func (s *Server) requireCubeOwner(w http.ResponseWriter, r *http.Request, cubeID uuid.UUID) bool {
	caller := appctx.From(r.Context())
	if caller.IsAdmin() {
		return true
	}
	owner, err := s.members.IsCubeOwner(r.Context(), cubeID, caller.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check cube ownership")
		return false
	}
	if owner {
		return true
	}
	member, err := s.members.IsCubeMember(r.Context(), cubeID, caller.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check cube access")
		return false
	}
	if member {
		writeErr(w, http.StatusForbidden, "only the cube owner may do that")
	} else {
		writeErr(w, http.StatusNotFound, "cube not found")
	}
	return false
}

// cubeParamAccess reads ?cube= and gates read access in one step, for the analytics
// and card handlers that scope by query rather than path.
func (s *Server) cubeParamAccess(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := cubeParam(w, r)
	if !ok {
		return uuid.Nil, false
	}
	if !s.requireCubeAccess(w, r, id) {
		return uuid.Nil, false
	}
	return id, true
}
