package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, len(users))
	for i, u := range users {
		out[i] = u.Public()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	u, err := s.store.GetUserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		writeErr(w, statusForStoreErr(err), "user not found")
		return
	}
	// Owner or admin sees the full record (incl. email); others get the public view.
	if appctx.From(r.Context()).CanMutateOwned(u.ID) {
		writeJSON(w, http.StatusOK, u)
		return
	}
	writeJSON(w, http.StatusOK, u.Public())
}

func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	caller := appctx.From(r.Context())
	if !caller.CanMutateOwned(id) {
		writeErr(w, http.StatusForbidden, "not allowed")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "user not found")
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
		AvatarURL   *string `json:"avatar_url"`
		Role        *string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.DisplayName != nil {
		u.DisplayName = *req.DisplayName
	}
	if req.Bio != nil {
		u.Bio = req.Bio
	}
	if req.AvatarURL != nil {
		u.AvatarURL = req.AvatarURL
	}
	if req.Role != nil {
		// Only admins may change roles.
		if !caller.IsAdmin() {
			writeErr(w, http.StatusForbidden, "only admins may change role")
			return
		}
		u.Role = *req.Role
	}
	if err := s.store.UpdateUserProfile(r.Context(), u); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update user")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "could not delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
