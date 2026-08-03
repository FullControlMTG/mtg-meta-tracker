package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// handleListCubeMembers returns a cube's members — the pool the deck-owner picker
// draws from. Any member may read it.
func (s *Server) handleListCubeMembers(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !s.requireCubeAccess(w, r, id) {
		return
	}
	members, err := s.store.ListCubeMembers(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list members")
		return
	}
	out := make([]map[string]any, len(members))
	for i, m := range members {
		out[i] = m.Public()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleInviteToCube invites an existing user (by username) to a cube. Owner/admin
// only. There is no public signup, so the invitee already has an account.
func (s *Server) handleInviteToCube(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !s.requireCubeOwner(w, r, id) {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	invitee, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no such user")
		return
	}
	if member, _ := s.store.IsCubeMember(r.Context(), id, invitee.ID); member {
		writeErr(w, http.StatusConflict, "already a member")
		return
	}
	if _, err := s.store.CreateInvite(r.Context(), id, invitee.ID, appctx.From(r.Context()).UserID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "invited"})
}

// handleListCubeInvites returns a cube's outstanding invites, for the owner's panel.
func (s *Server) handleListCubeInvites(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !s.requireCubeOwner(w, r, id) {
		return
	}
	invites, err := s.store.ListCubeInvites(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list invites")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

// handleRemoveCubeMember drops a member (owner/admin). The owner cannot be removed.
func (s *Server) handleRemoveCubeMember(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !s.requireCubeOwner(w, r, id) {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := s.store.RemoveCubeMember(r.Context(), id, userID); err != nil {
		msg := "could not remove member"
		if err == store.ErrConflict {
			msg = "cannot remove the owner; transfer ownership first"
		}
		writeErr(w, statusForStoreErr(err), msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMyInvites returns the caller's pending invites, for their dashboard.
func (s *Server) handleMyInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.store.ListUserInvites(r.Context(), appctx.From(r.Context()).UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list invites")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

// handleAcceptInvite / handleDeclineInvite: only the invitee may respond.
func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	s.respondToInvite(w, r, true)
}

func (s *Server) handleDeclineInvite(w http.ResponseWriter, r *http.Request) {
	s.respondToInvite(w, r, false)
}

func (s *Server) respondToInvite(w http.ResponseWriter, r *http.Request, accept bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	invitee, err := s.store.GetInviteRecipient(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "invite not found")
		return
	}
	if invitee != appctx.From(r.Context()).UserID {
		writeErr(w, http.StatusForbidden, "not your invite")
		return
	}
	if accept {
		err = s.store.AcceptInvite(r.Context(), id)
	} else {
		err = s.store.DeclineInvite(r.Context(), id)
	}
	if err != nil {
		writeErr(w, statusForStoreErr(err), "could not respond to invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
