package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/config"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/decklist"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/images"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/revalidate"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

type Server struct {
	store    *store.Store
	cfg      config.Config
	resolver *decklist.Resolver
	images   *images.Cache
}

func New(s *store.Store, cfg config.Config, resolver *decklist.Resolver, imgs *images.Cache) *Server {
	return &Server{
		store:    s,
		cfg:      cfg,
		resolver: resolver,
		images:   imgs,
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(s.resolveCaller)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/logout", s.handleLogout)
		r.Get("/auth/me", s.handleMe)
		r.Post("/auth/accept-invite", s.handleAcceptInvite)

		r.Get("/users", s.handleListUsers)
		r.Get("/users/{username}", s.handleGetUser)
		r.With(s.requireAuth).Patch("/users/{id}", s.handlePatchUser)
		r.With(s.requireAdmin).Delete("/users/{id}", s.handleDeleteUser)

		r.Get("/cubes", s.handleListCubes)
		r.Get("/cubes/{id}", s.handleGetCube)
		r.Get("/cubes/{id}/cards", s.handleGetCubeCards)

		r.Get("/cards/{id}/image", s.handleCardImage)

		r.Get("/decklists", s.handleListDecklists)
		r.Get("/decklists/{id}", s.handleGetDecklist)
		r.With(s.requireAuth).Post("/decklists", s.handleCreateDecklist)
		r.With(s.requireAuth).Patch("/decklists/{id}", s.handlePatchDecklist)
		r.With(s.requireAuth).Patch("/decklists/{id}/record", s.handlePatchDecklistRecord)
		r.With(s.requireAuth).Delete("/decklists/{id}", s.handleDeleteDecklist)
		r.With(s.requireAuth).Post("/decklists/infer-colors", s.handleInferColors)

		r.Get("/analytics/overview", s.handleAnalyticsOverview)
		r.Get("/analytics/colors", s.handleAnalyticsColors)
		r.Get("/analytics/cards", s.handleAnalyticsCards)
		r.Get("/analytics/pairs", s.handleAnalyticsPairs)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Post("/admin/invites", s.handleCreateInvite)
			r.Get("/admin/invites", s.handleListInvites)
			r.Delete("/admin/invites/{id}", s.handleDeleteInvite)

			r.Post("/admin/cubes", s.handleCreateCube)
			r.Patch("/admin/cubes/{id}", s.handlePatchCube)
			r.Delete("/admin/cubes/{id}", s.handleDeleteCube)
			r.Post("/admin/cubes/{id}/sync", s.handleSyncCube)
			r.Get("/admin/cubes/{id}/sync-status", s.handleCubeSyncStatus)

			r.Post("/admin/analytics/recompute", s.handleRecomputeAnalytics)
		})
	})
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
}

// revalidatePaths best-effort fires the Next.js revalidation webhook for the
// given paths without blocking the response. The request context is cancelled
// once we return, so it runs on a background context.
func (s *Server) revalidatePaths(paths []string) {
	go revalidate.Post(context.Background(), nil, s.cfg.RevalidateURL, s.cfg.RevalidateSecret, paths)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func statusForStoreErr(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
