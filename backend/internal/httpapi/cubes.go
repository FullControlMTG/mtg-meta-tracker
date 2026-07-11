package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/moxfield"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

func (s *Server) cubeView(r *http.Request, c *domain.Cube) map[string]any {
	count, _ := s.store.CountActiveCubeCards(r.Context(), c.ID)
	return map[string]any{"cube": c, "card_count": count}
}

func (s *Server) enqueueCubeSync(r *http.Request, id uuid.UUID) {
	_ = s.store.EnqueueJob(r.Context(), "sync_cube",
		map[string]string{"cube_id": id.String()}, "sync_cube:"+id.String())
}

// eqStrPtr reports whether two optional strings are equal, treating nil as absent.
func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Server) handleListCubes(w http.ResponseWriter, r *http.Request) {
	cubes, err := s.store.ListCubes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list cubes")
		return
	}
	out := make([]map[string]any, len(cubes))
	for i := range cubes {
		out[i] = s.cubeView(r, &cubes[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCube(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := s.store.GetCube(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	writeJSON(w, http.StatusOK, s.cubeView(r, c))
}

func (s *Server) handleGetCubeCards(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.store.GetCube(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	cards, err := s.store.ListCubeCards(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list cube cards")
		return
	}
	if cards == nil {
		cards = []store.CubeCardView{}
	}
	writeJSON(w, http.StatusOK, cards)
}

// allowedImageVariants gates the ?v= query so it maps only to real DB columns.
var allowedImageVariants = map[string]bool{"small": true, "normal": true, "art_crop": true}

// handleCardImage serves a card image from the on-disk cache, downloading it
// from Scryfall on a miss. Images are immutable per URL, so it advertises a
// long, immutable cache lifetime to browsers and the Next.js optimizer.
func (s *Server) handleCardImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	variant := r.URL.Query().Get("v")
	if variant == "" {
		variant = "normal"
	}
	if !allowedImageVariants[variant] {
		writeErr(w, http.StatusBadRequest, "invalid variant")
		return
	}

	src, err := s.store.GetCardImageURL(r.Context(), id, variant)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "image not found")
		return
	}
	path, err := s.images.Fetch(r.Context(), id.String()+"-"+variant, src)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not fetch image")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func (s *Server) handleCreateCube(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		MoxfieldURL string `json:"moxfield_url"`
		Description string `json:"description"`
		CardList    string `json:"card_list"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	c := &domain.Cube{Name: req.Name}
	if pid := moxfield.ParsePublicID(strings.TrimSpace(req.MoxfieldURL)); pid != "" {
		c.MoxfieldPublicID = &pid
	}
	if req.Description != "" {
		c.Description = &req.Description
	}
	if list := strings.TrimSpace(req.CardList); list != "" {
		c.CardList = &list
	}
	if err := s.store.CreateCube(r.Context(), c); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create cube")
		return
	}
	// Build the pool from the pasted list, if any. Mark it queued up front so the
	// admin page can follow the run from the moment it creates the cube — without
	// a row, sync-status reports "none" (never synced) and the poller would give
	// up before the worker (~2s) ever claims the job.
	if c.CardList != nil {
		_ = s.store.BeginCubeSyncProgress(r.Context(), c.ID, "queued")
		s.enqueueCubeSync(r, c.ID)
	}
	// Bust the public cube listing so the new cube surfaces promptly rather than
	// waiting out the ISR window. (A Moxfield-backed cube will also revalidate
	// its own page once the sync's analytics recompute finishes.)
	s.revalidatePaths([]string{"/", "/cubes"})
	writeJSON(w, http.StatusCreated, s.cubeView(r, c))
}

func (s *Server) handlePatchCube(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := s.store.GetCube(r.Context(), id)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	var req struct {
		Name        *string `json:"name"`
		MoxfieldURL *string `json:"moxfield_url"`
		Description *string `json:"description"`
		CardList    *string `json:"card_list"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	listChanged := false
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.MoxfieldURL != nil {
		// Moxfield URL is display-only metadata; changing it does not rebuild the pool.
		if pid := moxfield.ParsePublicID(strings.TrimSpace(*req.MoxfieldURL)); pid == "" {
			c.MoxfieldPublicID = nil
		} else {
			c.MoxfieldPublicID = &pid
		}
	}
	if req.Description != nil {
		c.Description = req.Description
	}
	if req.CardList != nil {
		list := strings.TrimSpace(*req.CardList)
		var next *string
		if list != "" {
			next = &list
		}
		if !eqStrPtr(c.CardList, next) {
			c.CardList = next
			listChanged = true
		}
	}
	if err := s.store.UpdateCube(r.Context(), c); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update cube")
		return
	}
	if listChanged {
		// Mark the sync queued before enqueueing, as handleSyncCube does. Until the
		// worker claims the job (~2s), sync-status would otherwise still serve the
		// previous run's *done* row — so an admin who just edited the list to fix a
		// typo would see the old card count and the old unresolved names reported as
		// the result of their edit. Leaving "done" is what suppresses that; the
		// resolve then overwrites the counters and the unresolved list.
		_ = s.store.BeginCubeSyncProgress(r.Context(), c.ID, "queued")
		s.enqueueCubeSync(r, c.ID)
	}
	// Bust the public cube listing and this cube's page so edits surface promptly
	// rather than waiting out the ISR window. (A list change also revalidates
	// again once its sync's analytics recompute finishes.)
	s.revalidatePaths([]string{"/", "/cubes", "/cubes/" + c.ID.String()})
	writeJSON(w, http.StatusOK, s.cubeView(r, c))
}

func (s *Server) handleSyncCube(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.store.GetCube(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	// Force a full re-resolve even when the list is unchanged.
	_ = s.store.ClearCubeContentHash(r.Context(), id)
	// Mark the sync queued right away so the admin page shows immediate feedback
	// before the worker (which polls every ~2s) claims the job.
	_ = s.store.BeginCubeSyncProgress(r.Context(), id, "queued")
	s.enqueueCubeSync(r, id)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync enqueued"})
}

// handleCubeSyncStatus returns the live progress of the "Sync Scryfall images"
// action for a cube. A cube that has never been synced has no row; that is
// reported as {"status":"none"} rather than a 404 so the admin page can treat
// it as a non-error idle state.
func (s *Server) handleCubeSyncStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := s.store.GetCubeSyncProgress(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not read sync status")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteCube(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteCube(r.Context(), id); err != nil {
		writeErr(w, statusForStoreErr(err), "could not delete cube")
		return
	}
	s.revalidatePaths([]string{"/", "/cubes", "/cubes/" + id.String()})
	w.WriteHeader(http.StatusNoContent)
}
