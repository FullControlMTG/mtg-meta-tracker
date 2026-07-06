package httpapi

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/auth"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const inviteTTL = 7 * 24 * time.Hour

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeErr(w, http.StatusBadRequest, "email required")
		return
	}
	if req.Role == "" {
		req.Role = domain.RoleUser
	}
	if req.Role != domain.RoleUser && req.Role != domain.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "role must be user or admin")
		return
	}
	raw, hash, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token gen failed")
		return
	}
	caller := appctx.From(r.Context())
	invitedBy := caller.UserID
	inv := &domain.Invite{
		Email:     req.Email,
		Role:      req.Role,
		TokenHash: hash,
		InvitedBy: &invitedBy,
		ExpiresAt: time.Now().Add(inviteTTL),
	}
	if err := s.store.CreateInvite(r.Context(), inv); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	acceptURL := s.cfg.AppBaseURL + "/accept-invite?token=" + url.QueryEscape(raw)
	writeJSON(w, http.StatusCreated, map[string]any{
		"invite":     inv,
		"token":      raw, // shown once so the admin can share the link
		"accept_url": acceptURL,
	})
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.store.ListPendingInvites(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list invites")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

func (s *Server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteInvite(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "could not delete invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
