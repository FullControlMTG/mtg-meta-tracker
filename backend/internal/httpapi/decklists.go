package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/decklist"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

func (s *Server) enqueueRecompute(r *http.Request, cubeID uuid.UUID, trigger string) {
	_ = s.store.EnqueueJob(r.Context(), "recompute_analytics",
		map[string]string{"cube_id": cubeID.String(), "trigger": trigger},
		"recompute:"+cubeID.String())
}

func (s *Server) decklistView(r *http.Request, d *domain.Decklist) map[string]any {
	cards, _ := s.store.GetDecklistCardsView(r.Context(), d.ID)
	view := map[string]any{
		"decklist":     d,
		"color_string": domain.ColorIdentity(d.ColorIdentity).String(),
		"cards":        cards,
	}
	if u, err := s.store.GetUserByID(r.Context(), d.UserID); err == nil {
		view["user"] = u.Public()
	}
	return view
}

func (s *Server) handleListDecklists(w http.ResponseWriter, r *http.Request) {
	var f store.DecklistFilter
	if v := r.URL.Query().Get("cube"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid cube id")
			return
		}
		f.CubeID = &id
	}
	if v := r.URL.Query().Get("user"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid user id")
			return
		}
		f.UserID = &id
	}
	decks, err := s.store.ListDecklists(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list decklists")
		return
	}
	out := make([]map[string]any, len(decks))
	for i := range decks {
		out[i] = map[string]any{
			"decklist":     decks[i],
			"color_string": domain.ColorIdentity(decks[i].ColorIdentity).String(),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDecklist(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := s.store.GetDecklist(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "decklist not found")
		return
	}
	writeJSON(w, http.StatusOK, s.decklistView(r, d))
}

func (s *Server) handleCreateDecklist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CubeID      string `json:"cube_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Archetype   string `json:"archetype"`
		SourceURL   string `json:"source_url"`
		DecklistRaw string `json:"decklist_raw"`
		Status      string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cubeID, err := uuid.Parse(strings.TrimSpace(req.CubeID))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid cube id")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if strings.TrimSpace(req.DecklistRaw) == "" {
		writeErr(w, http.StatusBadRequest, "decklist_raw required")
		return
	}
	status := domain.StatusActive
	if req.Status != "" {
		if !validStatus(req.Status) {
			writeErr(w, http.StatusBadRequest, "invalid status")
			return
		}
		status = req.Status
	}

	resolved, err := s.resolver.Resolve(r.Context(), cubeID, decklist.ParseList(req.DecklistRaw))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not resolve cards: "+err.Error())
		return
	}

	caller := appctx.From(r.Context())
	d := &domain.Decklist{
		CubeID:        cubeID,
		UserID:        caller.UserID,
		Name:          req.Name,
		ColorIdentity: resolved.ColorIdentity,
		DecklistRaw:   req.DecklistRaw,
		CardCount:     countMain(resolved.Cards),
		Status:        status,
	}
	if req.Description != "" {
		d.Description = &req.Description
	}
	if req.Archetype != "" {
		d.Archetype = &req.Archetype
	}
	if req.SourceURL != "" {
		d.SourceURL = &req.SourceURL
	}
	if err := s.store.CreateDecklist(r.Context(), d, resolved.Cards); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create decklist")
		return
	}
	s.enqueueRecompute(r, cubeID, "deck_created")
	writeJSON(w, http.StatusCreated, s.decklistView(r, d))
}

func (s *Server) handlePatchDecklist(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := s.store.GetDecklist(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "decklist not found")
		return
	}
	if !appctx.From(r.Context()).CanMutateOwned(d.UserID) {
		writeErr(w, http.StatusForbidden, "not allowed")
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Archetype   *string `json:"archetype"`
		SourceURL   *string `json:"source_url"`
		DecklistRaw *string `json:"decklist_raw"`
		Status      *string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != nil {
		d.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		d.Description = req.Description
	}
	if req.Archetype != nil {
		d.Archetype = req.Archetype
	}
	if req.SourceURL != nil {
		d.SourceURL = req.SourceURL
	}
	if req.Status != nil {
		if !validStatus(*req.Status) {
			writeErr(w, http.StatusBadRequest, "invalid status")
			return
		}
		d.Status = *req.Status
	}

	var cards []domain.DecklistCard
	if req.DecklistRaw != nil && *req.DecklistRaw != d.DecklistRaw {
		if strings.TrimSpace(*req.DecklistRaw) == "" {
			writeErr(w, http.StatusBadRequest, "decklist_raw cannot be empty")
			return
		}
		resolved, err := s.resolver.Resolve(r.Context(), d.CubeID, decklist.ParseList(*req.DecklistRaw))
		if err != nil {
			writeErr(w, http.StatusBadGateway, "could not resolve cards: "+err.Error())
			return
		}
		d.DecklistRaw = *req.DecklistRaw
		d.ColorIdentity = resolved.ColorIdentity
		d.CardCount = countMain(resolved.Cards)
		cards = resolved.Cards
	}

	if err := s.store.UpdateDecklist(r.Context(), d, cards); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update decklist")
		return
	}
	s.enqueueRecompute(r, d.CubeID, "deck_updated")
	writeJSON(w, http.StatusOK, s.decklistView(r, d))
}

func (s *Server) handlePatchDecklistRecord(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := s.store.GetDecklist(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "decklist not found")
		return
	}
	if !appctx.From(r.Context()).CanMutateOwned(d.UserID) {
		writeErr(w, http.StatusForbidden, "not allowed")
		return
	}
	var req struct {
		GamesPlayed int        `json:"games_played"`
		Wins        int        `json:"wins"`
		Losses      int        `json:"losses"`
		Draws       int        `json:"draws"`
		Placement   *int       `json:"placement"`
		EventName   *string    `json:"event_name"`
		PlayedAt    *time.Time `json:"played_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.GamesPlayed < 0 || req.Wins < 0 || req.Losses < 0 || req.Draws < 0 {
		writeErr(w, http.StatusBadRequest, "record values must be non-negative")
		return
	}
	if req.Wins+req.Losses+req.Draws > req.GamesPlayed {
		writeErr(w, http.StatusBadRequest, "wins+losses+draws exceeds games_played")
		return
	}
	rec := store.DecklistRecord{
		GamesPlayed: req.GamesPlayed,
		Wins:        req.Wins,
		Losses:      req.Losses,
		Draws:       req.Draws,
		Placement:   req.Placement,
		EventName:   req.EventName,
		PlayedAt:    req.PlayedAt,
	}
	if err := s.store.UpdateDecklistRecord(r.Context(), id, rec); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update record")
		return
	}
	s.enqueueRecompute(r, d.CubeID, "record_updated")
	d, _ = s.store.GetDecklist(r.Context(), id)
	writeJSON(w, http.StatusOK, s.decklistView(r, d))
}

func (s *Server) handleDeleteDecklist(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := s.store.GetDecklist(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "decklist not found")
		return
	}
	if !appctx.From(r.Context()).CanMutateOwned(d.UserID) {
		writeErr(w, http.StatusForbidden, "not allowed")
		return
	}
	if err := s.store.DeleteDecklist(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "could not delete decklist")
		return
	}
	s.enqueueRecompute(r, d.CubeID, "deck_updated")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInferColors(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CubeID      string `json:"cube_id"`
		DecklistRaw string `json:"decklist_raw"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cubeID, err := uuid.Parse(strings.TrimSpace(req.CubeID))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid cube id")
		return
	}
	parsed := decklist.ParseList(req.DecklistRaw)
	resolved, err := s.resolver.Resolve(r.Context(), cubeID, parsed)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not resolve cards: "+err.Error())
		return
	}
	var resolvedNames []string
	for _, c := range resolved.Cards {
		if c.IsResolved {
			resolvedNames = append(resolvedNames, c.CardName)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"color_identity": resolved.ColorIdentity,
		"color_string":   domain.ColorIdentity(resolved.ColorIdentity).String(),
		"resolved":       resolvedNames,
		"unresolved":     resolved.Unresolved,
	})
}

func validStatus(s string) bool {
	return s == domain.StatusDraft || s == domain.StatusActive || s == domain.StatusArchived
}

func countMain(cards []domain.DecklistCard) int {
	n := 0
	for _, c := range cards {
		if c.Board == domain.BoardMain {
			n += c.Quantity
		}
	}
	return n
}
