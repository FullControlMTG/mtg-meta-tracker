package httpapi

import (
	"fmt"
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
		"decklist":      d,
		"color_string":  domain.ColorIdentity(d.ColorIdentity).String(),
		"splash_string": domain.ColorIdentity(d.SplashColors).String(),
		"cards":         cards,
	}
	if u, err := s.store.GetUserByID(r.Context(), d.UserID); err == nil {
		view["user"] = u.Public()
	}
	return view
}

// resolveOwner interprets a user_id sent with a deck save. An empty value means
// "leave it alone" and yields current. Anyone may own their own decks; only an
// admin may hand a deck to someone else. The target must exist — decklists.user_id
// is a FK, so a bad id would otherwise surface as a 500.
func (s *Server) resolveOwner(r *http.Request, requested string, current uuid.UUID) (uuid.UUID, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return current, nil
	}
	id, err := uuid.Parse(requested)
	if err != nil {
		return uuid.Nil, apiError{http.StatusBadRequest, "invalid user id"}
	}
	if id == current {
		return current, nil
	}
	if !appctx.From(r.Context()).IsAdmin() {
		return uuid.Nil, apiError{http.StatusForbidden, "only admins may set the deck owner"}
	}
	if _, err := s.store.GetUserByID(r.Context(), id); err != nil {
		return uuid.Nil, apiError{http.StatusBadRequest, "unknown user"}
	}
	return id, nil
}

// revalidateOwnerChange refreshes both profile pages after a deck changes hands.
// They are ISR (revalidate = 3600), so without this the deck lists the wrong
// person for up to an hour.
func (s *Server) revalidateOwnerChange(r *http.Request, from, to uuid.UUID) {
	if from == to {
		return
	}
	var paths []string
	for _, id := range []uuid.UUID{from, to} {
		if u, err := s.store.GetUserByID(r.Context(), id); err == nil {
			paths = append(paths, "/users/"+u.Username)
		}
	}
	if len(paths) > 0 {
		s.revalidatePaths(paths)
	}
}

// withUnresolved reports the names a save could not match, so a card that was
// silently dropped is visible in the response that dropped it. Only /infer-colors
// used to say anything, which meant an actual save could quietly lose cards.
// Always a list, never null, so the client can render it without a nil check.
func withUnresolved(view map[string]any, unresolved []string) map[string]any {
	if unresolved == nil {
		unresolved = []string{}
	}
	view["unresolved"] = unresolved
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
	// Owner and cube travel with the list so the deck table can filter on them
	// (frontend lib/deckQuery.ts) — a `user:` or `cube:` term has nothing to match
	// against a bare user_id. Both are read whole rather than joined per deck: a
	// playgroup has a handful of each, and neither list is worth an N+1.
	users := map[uuid.UUID]map[string]any{}
	if list, err := s.store.ListUsers(r.Context()); err == nil {
		for _, u := range list {
			users[u.ID] = u.Public()
		}
	}
	cubes := map[uuid.UUID]string{}
	if list, err := s.store.ListCubes(r.Context()); err == nil {
		for _, c := range list {
			cubes[c.ID] = c.Name
		}
	}
	out := make([]map[string]any, len(decks))
	for i := range decks {
		item := map[string]any{
			"decklist":      decks[i],
			"color_string":  domain.ColorIdentity(decks[i].ColorIdentity).String(),
			"splash_string": domain.ColorIdentity(decks[i].SplashColors).String(),
		}
		if u, ok := users[decks[i].UserID]; ok {
			item["user"] = u
		}
		if name, ok := cubes[decks[i].CubeID]; ok {
			item["cube_name"] = name
		}
		out[i] = item
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
		CubeID string `json:"cube_id"`
		// Owner. Empty means the caller; only an admin may name someone else.
		UserID      string `json:"user_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Archetype   string `json:"archetype"`
		SourceURL   string `json:"source_url"`
		DecklistRaw string `json:"decklist_raw"`
		Status      string `json:"status"`
		// The day the deck was played, "2006-01-02". Omitted means today.
		PlayedAt string `json:"played_at"`
		// Optional record, if the deck was already played before listing.
		GamesPlayed *int    `json:"games_played"`
		Wins        int     `json:"wins"`
		Losses      int     `json:"losses"`
		EventName   *string `json:"event_name"`
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
	playedAt, err := s.parsePlayedAt(req.PlayedAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	resolved, err := s.resolver.Resolve(r.Context(), cubeID, decklist.ParseList(req.DecklistRaw))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not resolve cards: "+err.Error())
		return
	}

	caller := appctx.From(r.Context())
	ownerID, err := s.resolveOwner(r, req.UserID, caller.UserID)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	d := &domain.Decklist{
		CubeID:        cubeID,
		UserID:        ownerID,
		Name:          req.Name,
		ColorIdentity: resolved.ColorIdentity,
		SplashColors:  resolved.SplashColors,
		DecklistRaw:   req.DecklistRaw,
		CardCount:     countMain(resolved.Cards),
		Status:        status,
		PlayedAt:      playedAt,
	}
	if req.Description != "" {
		d.Description = &req.Description
	}
	if req.Archetype != "" {
		if !validArchetype(req.Archetype) {
			writeErr(w, http.StatusBadRequest, "invalid archetype")
			return
		}
		d.Archetype = &req.Archetype
	}
	if req.SourceURL != "" {
		d.SourceURL = &req.SourceURL
	}
	// Optional record supplied at create time.
	hasRecord := req.GamesPlayed != nil || req.Wins > 0 || req.Losses > 0 || req.EventName != nil
	var rec store.DecklistRecord
	if hasRecord {
		rec, err = buildRecord(req.GamesPlayed, req.Wins, req.Losses, req.EventName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := s.store.CreateDecklist(r.Context(), d, resolved.Cards); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create decklist")
		return
	}
	if hasRecord {
		if err := s.store.UpdateDecklistRecord(r.Context(), d.ID, rec); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not save record")
			return
		}
		d, _ = s.store.GetDecklist(r.Context(), d.ID)
	}
	s.enqueueRecompute(r, cubeID, "deck_created")
	writeJSON(w, http.StatusCreated, withUnresolved(s.decklistView(r, d), resolved.Unresolved))
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
		// Reassigns the deck. Admin-only; see resolveOwner.
		UserID      *string `json:"user_id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Archetype   *string `json:"archetype"`
		SourceURL   *string `json:"source_url"`
		DecklistRaw *string `json:"decklist_raw"`
		Status      *string `json:"status"`
		// "2006-01-02". Absent leaves the date alone; it can be changed, never cleared.
		PlayedAt *string `json:"played_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	prevOwner := d.UserID
	if req.UserID != nil {
		owner, err := s.resolveOwner(r, *req.UserID, d.UserID)
		if err != nil {
			writeAPIErr(w, err)
			return
		}
		d.UserID = owner
	}
	if req.Name != nil {
		d.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		d.Description = req.Description
	}
	if req.Archetype != nil {
		switch {
		case *req.Archetype == "": // the form's "none" option clears the tag
			d.Archetype = nil
		case !validArchetype(*req.Archetype):
			writeErr(w, http.StatusBadRequest, "invalid archetype")
			return
		default:
			d.Archetype = req.Archetype
		}
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
	if req.PlayedAt != nil {
		// Unlike create, an empty string here is not "today" — it is a form sending a
		// cleared date input, and the deck's date is not nullable. Reject it.
		played, err := s.parsePlayedAt(*req.PlayedAt)
		if err != nil || *req.PlayedAt == "" {
			writeErr(w, http.StatusBadRequest, "played_at must be a date (YYYY-MM-DD)")
			return
		}
		d.PlayedAt = played
	}

	var cards []domain.DecklistCard
	var unresolved []string
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
		d.SplashColors = resolved.SplashColors
		d.CardCount = countMain(resolved.Cards)
		cards = resolved.Cards
		unresolved = resolved.Unresolved
	}

	if err := s.store.UpdateDecklist(r.Context(), d, cards); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update decklist")
		return
	}
	s.enqueueRecompute(r, d.CubeID, "deck_updated")
	s.revalidateOwnerChange(r, prevOwner, d.UserID)
	writeJSON(w, http.StatusOK, withUnresolved(s.decklistView(r, d), unresolved))
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
	// The date the deck was played is not part of the record — it is a deck field,
	// patched through PATCH /decklists/{id}. Sending it here does nothing.
	var req struct {
		GamesPlayed *int    `json:"games_played"`
		Wins        int     `json:"wins"`
		Losses      int     `json:"losses"`
		EventName   *string `json:"event_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec, err := buildRecord(req.GamesPlayed, req.Wins, req.Losses, req.EventName)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
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
	// The recompute's own revalidation derives its paths from the cube's decklists,
	// which no longer include this one, so the deck's page and its owner's profile
	// (both ISR, revalidate = 3600) would keep serving the deleted deck for an hour.
	paths := []string{"/decks/" + id.String()}
	if u, err := s.store.GetUserByID(r.Context(), d.UserID); err == nil {
		paths = append(paths, "/users/"+u.Username)
	}
	s.revalidatePaths(paths)
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
		"splash_colors":  resolved.SplashColors,
		"splash_string":  domain.ColorIdentity(resolved.SplashColors).String(),
		"resolved":       resolvedNames,
		"unresolved":     resolved.Unresolved,
	})
}

// today is the current calendar day in the playgroup's timezone (config.Timezone,
// America/Los_Angeles). The server itself runs in UTC, where the day turns over
// mid-afternoon locally — a deck uploaded after 5pm would be dated tomorrow.
func (s *Server) today() time.Time {
	now := time.Now().In(s.cfg.Timezone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// parsePlayedAt reads the date a deck was played. Empty means today, which the server
// decides — a browser's idea of the day is its own timezone's, and the deck belongs to
// a playgroup that sits in one.
//
// The wire format is a plain calendar day, "2006-01-02", which is what an <input
// type="date"> submits and what a DATE column stores; RFC3339 is accepted too, since
// that is how the field is *served* and a client may well hand back what it was given.
// The result is midnight UTC, so the calendar day survives the round trip whatever
// timezone anything downstream keeps.
func (s *Server) parsePlayedAt(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return s.today(), nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("played_at must be a date (YYYY-MM-DD)")
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

// handleToday serves the server's current date so a form's date picker can open on
// the same day the server would have chosen. Public: it is a clock, not data.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"date":     s.today().Format("2006-01-02"),
		"timezone": s.cfg.Timezone.String(),
	})
}

// buildRecord validates the optional record fields and assembles a store record.
// games_played defaults to wins+losses when the caller omits it.
func buildRecord(gamesPlayed *int, wins, losses int, event *string) (store.DecklistRecord, error) {
	gp := wins + losses
	if gamesPlayed != nil {
		gp = *gamesPlayed
	}
	if gp < 0 || wins < 0 || losses < 0 {
		return store.DecklistRecord{}, fmt.Errorf("record values must be non-negative")
	}
	if wins+losses > gp {
		return store.DecklistRecord{}, fmt.Errorf("wins+losses exceeds games_played")
	}
	return store.DecklistRecord{
		GamesPlayed: gp,
		Wins:        wins,
		Losses:      losses,
		EventName:   event,
	}, nil
}

func validStatus(s string) bool {
	return s == domain.StatusDraft || s == domain.StatusActive || s == domain.StatusArchived
}

func validArchetype(s string) bool {
	switch s {
	case domain.ArchetypeAggro, domain.ArchetypeControl, domain.ArchetypeMidrange,
		domain.ArchetypeTempo, domain.ArchetypeCombo:
		return true
	}
	return false
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
