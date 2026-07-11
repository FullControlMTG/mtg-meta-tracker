package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/auth"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const minPasswordLen = 8

// Onboarding is admin-driven: there is no open registration and no invite flow.
// The admin picks the username and the initial password and hands them over.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeErr(w, http.StatusBadRequest, "username required")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "password must be >= 8 chars")
		return
	}
	if req.Role == "" {
		req.Role = domain.RoleUser
	}
	if req.Role != domain.RoleUser && req.Role != domain.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "role must be user or admin")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	display := strings.TrimSpace(req.DisplayName)
	if display == "" {
		display = req.Username
	}
	u := &domain.User{
		Username:     req.Username,
		DisplayName:  display,
		Role:         req.Role,
		PasswordHash: &hash,
	}
	// Email is optional. Store NULL rather than "", which would collide with the
	// next account left blank under the UNIQUE constraint.
	if e := strings.TrimSpace(req.Email); e != "" {
		u.Email = &e
	}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		writeErr(w, http.StatusConflict, "username or email already taken")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// handleSetPassword serves both "change my password" and an admin resetting
// someone else's. Changing your own — admin or not — requires the current one;
// an admin resetting another account does not, so a locked-out user can be given
// a fresh password. Either way every existing session for the target is dropped,
// so an old password's sessions cannot outlive it.
func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "password must be >= 8 chars")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "user not found")
		return
	}
	self := caller.Owns(id)
	if self {
		if u.PasswordHash == nil {
			writeErr(w, http.StatusBadRequest, "account has no password set")
			return
		}
		ok, err := auth.VerifyPassword(*u.PasswordHash, req.CurrentPassword)
		if err != nil || !ok {
			writeErr(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), id, hash); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update password")
		return
	}
	if err := s.store.DeleteUserSessions(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not clear sessions")
		return
	}
	// Clearing sessions just logged the caller out of their own account; issue a
	// fresh one so changing your password does not bounce you to the login page.
	if self {
		if err := s.startSession(r.Context(), w, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not start session")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

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
