package httpapi

import (
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
	if err := s.store.CreateCube(r.Context(), c); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create cube")
		return
	}
	if c.MoxfieldPublicID != nil {
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
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	sourceChanged := false
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.MoxfieldURL != nil {
		pid := moxfield.ParsePublicID(strings.TrimSpace(*req.MoxfieldURL))
		if pid == "" {
			c.MoxfieldPublicID = nil
		} else if c.MoxfieldPublicID == nil || *c.MoxfieldPublicID != pid {
			c.MoxfieldPublicID = &pid
			sourceChanged = true
		}
	}
	if req.Description != nil {
		c.Description = req.Description
	}
	if err := s.store.UpdateCube(r.Context(), c); err != nil {
		writeErr(w, statusForStoreErr(err), "could not update cube")
		return
	}
	if sourceChanged {
		s.enqueueCubeSync(r, c.ID)
	}
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
	s.enqueueCubeSync(r, id)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync enqueued"})
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
