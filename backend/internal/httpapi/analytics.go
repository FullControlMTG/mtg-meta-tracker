package httpapi

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// cubeParam reads the required ?cube= query param.
func cubeParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.URL.Query().Get("cube"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cube query param required")
		return uuid.Nil, false
	}
	return id, true
}

// currentRun resolves the cube's current analytics run, writing 404 if none.
func (s *Server) currentRun(w http.ResponseWriter, r *http.Request, cubeID uuid.UUID) (uuid.UUID, bool) {
	run, err := s.store.GetCurrentRun(r.Context(), cubeID)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "no analytics available for this cube")
		return uuid.Nil, false
	}
	return run.ID, true
}

func (s *Server) handleAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	cubeID, ok := cubeParam(w, r)
	if !ok {
		return
	}
	run, err := s.store.GetCurrentRun(r.Context(), cubeID)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "no analytics available for this cube")
		return
	}
	meta, err := s.store.GetMetaSnapshot(r.Context(), run.ID)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "no snapshot for run")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "meta": meta})
}

func (s *Server) handleAnalyticsColors(w http.ResponseWriter, r *http.Request) {
	cubeID, ok := cubeParam(w, r)
	if !ok {
		return
	}
	runID, ok := s.currentRun(w, r, cubeID)
	if !ok {
		return
	}
	stats, err := s.store.ListColorStats(r.Context(), runID, r.URL.Query().Get("facet"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load color stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAnalyticsCards(w http.ResponseWriter, r *http.Request) {
	cubeID, ok := cubeParam(w, r)
	if !ok {
		return
	}
	runID, ok := s.currentRun(w, r, cubeID)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	stats, err := s.store.ListCardStats(r.Context(), runID, r.URL.Query().Get("sort"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load card stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAnalyticsPairs(w http.ResponseWriter, r *http.Request) {
	cubeID, ok := cubeParam(w, r)
	if !ok {
		return
	}
	cardID, err := uuid.Parse(r.URL.Query().Get("card"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "card query param required")
		return
	}
	runID, ok := s.currentRun(w, r, cubeID)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	pairs, err := s.store.ListCardPairs(r.Context(), runID, cardID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load pairs")
		return
	}
	writeJSON(w, http.StatusOK, pairs)
}

func (s *Server) handleRecomputeAnalytics(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CubeID string `json:"cube_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cubeID, err := uuid.Parse(req.CubeID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid cube id")
		return
	}
	if _, err := s.store.GetCube(r.Context(), cubeID); err != nil {
		writeErr(w, statusForStoreErr(err), "cube not found")
		return
	}
	s.enqueueRecompute(r, cubeID, "manual")
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "recompute enqueued"})
}
