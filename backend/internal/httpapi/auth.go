package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/auth"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// startSession creates a session row + cookie for a user.
func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, userID uuid.UUID) error {
	raw, hash, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(ctx, &domain.Session{ID: hash, UserID: userID, ExpiresAt: expires}); err != nil {
		return err
	}
	s.setSessionCookie(w, raw, expires)
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := s.store.GetUserByLogin(r.Context(), strings.TrimSpace(req.Login))
	if err != nil || u.PasswordHash == nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	ok, err := auth.VerifyPassword(*u.PasswordHash, req.Password)
	if err != nil || !ok {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := s.startSession(r.Context(), w, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start session")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(s.cfg.SessionCookieName); err == nil && ck.Value != "" {
		_ = s.store.DeleteSession(r.Context(), auth.HashToken(ck.Value))
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	caller := appctx.From(r.Context())
	if !caller.IsAuthenticated() {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), caller.UserID)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "user not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "username required and password must be >= 8 chars")
		return
	}
	inv, err := s.store.GetOpenInviteByTokenHash(r.Context(), auth.HashToken(req.Token))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired invite")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	display := req.DisplayName
	if display == "" {
		display = req.Username
	}
	u := &domain.User{
		Username:     req.Username,
		Email:        inv.Email,
		DisplayName:  display,
		Role:         inv.Role,
		PasswordHash: &hash,
	}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		writeErr(w, http.StatusConflict, "username or email already taken")
		return
	}
	_ = s.store.MarkInviteAccepted(r.Context(), inv.ID)
	if err := s.startSession(r.Context(), w, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start session")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}
