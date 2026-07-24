package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// A combo is at least a pair. The upper bound is not a rule of the game — it
// guards the admin form (and the previews under a deck) against a "combo" that is
// really a whole archetype's worth of cards typed into one row.
const (
	minComboCards = 2
	maxComboCards = 10
)

// comboRequest is the create/update payload. Both send the whole combo: the piece
// list has no stable identity to patch a member of, so an edit replaces it.
type comboRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CardIDs     []string `json:"card_ids"`
}

// parse validates the payload and returns it as a store input. Pieces are
// de-duplicated: naming a card twice is a slip in the form, not a two-of.
func (req comboRequest) parse() (store.ComboInput, error) {
	in := store.ComboInput{Name: strings.TrimSpace(req.Name)}
	if in.Name == "" {
		return in, apiError{http.StatusBadRequest, "name required"}
	}
	if d := strings.TrimSpace(req.Description); d != "" {
		in.Description = &d
	}
	seen := map[uuid.UUID]bool{}
	for _, raw := range req.CardIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return in, apiError{http.StatusBadRequest, "invalid card id"}
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		in.CardIDs = append(in.CardIDs, id)
	}
	if len(in.CardIDs) < minComboCards {
		return in, apiError{http.StatusBadRequest, "a combo needs at least two different cards"}
	}
	if len(in.CardIDs) > maxComboCards {
		return in, apiError{http.StatusBadRequest, "a combo may hold at most ten cards"}
	}
	return in, nil
}

// comboStoreErr maps the store's combo failures onto client errors. Anything
// else falls through to writeAPIErr's 500.
func comboStoreErr(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return apiError{http.StatusNotFound, "combo not found"}
	case errors.Is(err, store.ErrComboPieceNotInPool):
		return apiError{http.StatusBadRequest, "every combo card must be in the cube's pool"}
	case errors.Is(err, store.ErrComboNameTaken):
		return apiError{http.StatusConflict, "a combo with that name already exists in this cube"}
	}
	return err
}

// revalidateCubeDecks refreshes every deck page of a cube. Combos are matched at
// read time, so changing a definition changes what those pages say — and they are
// ISR (revalidate = 3600), which would otherwise sit on the old answer for an hour.
func (s *Server) revalidateCubeDecks(r *http.Request, cubeID uuid.UUID) {
	ids, err := s.store.ListCubeDecklistIDs(r.Context(), cubeID)
	if err != nil || len(ids) == 0 {
		return
	}
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		paths = append(paths, "/decks/"+id.String())
	}
	s.revalidatePaths(paths)
}

func (s *Server) handleListCombos(w http.ResponseWriter, r *http.Request) {
	cubeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid cube id")
		return
	}
	combos, err := s.store.ListCombos(r.Context(), cubeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list combos")
		return
	}
	writeJSON(w, http.StatusOK, combos)
}

func (s *Server) handleCreateCombo(w http.ResponseWriter, r *http.Request) {
	cubeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid cube id")
		return
	}
	if _, err := s.store.GetCube(r.Context(), cubeID); err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	var req comboRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in, err := req.parse()
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	id, err := s.store.CreateCombo(r.Context(), cubeID, in)
	if err != nil {
		writeAPIErr(w, comboStoreErr(err))
		return
	}
	combo, err := s.store.GetCombo(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read back combo")
		return
	}
	s.revalidateCubeDecks(r, cubeID)
	writeJSON(w, http.StatusCreated, combo)
}

func (s *Server) handlePatchCombo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.store.GetCombo(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "combo not found")
		return
	}
	var req comboRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in, err := req.parse()
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	if err := s.store.UpdateCombo(r.Context(), id, in); err != nil {
		writeAPIErr(w, comboStoreErr(err))
		return
	}
	combo, err := s.store.GetCombo(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read back combo")
		return
	}
	s.revalidateCubeDecks(r, existing.CubeID)
	writeJSON(w, http.StatusOK, combo)
}

func (s *Server) handleDeleteCombo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.store.GetCombo(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "combo not found")
		return
	}
	if err := s.store.DeleteCombo(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "could not delete combo")
		return
	}
	s.revalidateCubeDecks(r, existing.CubeID)
	w.WriteHeader(http.StatusNoContent)
}
