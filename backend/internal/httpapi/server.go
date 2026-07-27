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

	// Every route lives in exactly one of the three tiers below. New routes
	// join a tier — they do not sit outside — so a reviewer opening this file
	// can tell at a glance what a caller has to prove to hit each endpoint.
	//
	//   Public         — no session required. Reserved for auth handshakes,
	//                    liveness, the server's own calendar, and the raw
	//                    card-image proxy. Card art is public information
	//                    (Scryfall serves the same bytes to the world), and
	//                    the URL leaks nothing beyond "this card id exists
	//                    in the cache."
	//   Authenticated  — signed-in playgroup member. Everything derived from
	//                    or scoped to the playgroup lives here: cube pools,
	//                    deck lists, analytics, user directory, per-card
	//                    stats. This is the tier the earlier public reads
	//                    accidentally lived in.
	//   Admin          — moderation and configuration. Requires an admin
	//                    role in addition to being authenticated.
	//
	// Object-level ownership checks (e.g., "only the deck's author may edit
	// it") stay inside the handler via appctx.Caller.Owns / CanMutateOwned.
	// Middleware only knows "authenticated" and "admin"; it cannot know
	// which deck a request is about.
	r.Route("/api", func(r chi.Router) {
		// --- Public ---
		r.Group(func(r chi.Router) {
			r.Get("/health", s.handleHealth)
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			// The card-image proxy is <img src> fodder — same bytes Scryfall
			// serves publicly, cached at this origin so authenticated pages
			// don't need to embed cookies in image URLs.
			r.Get("/cards/{id}/image", s.handleCardImage)
			// The playgroup's calendar day is server-only knowledge but not
			// user-scoped; the login form asks for it before there is a user.
			r.Get("/today", s.handleToday)
		})

		// --- Authenticated ---
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)

			// Users
			r.Get("/users", s.handleListUsers)
			r.Get("/users/{username}", s.handleGetUser)
			r.Patch("/users/{id}", s.handlePatchUser)
			r.Post("/users/{id}/password", s.handleSetPassword)

			// Cubes
			r.Get("/cubes", s.handleListCubes)
			r.Get("/cubes/{id}", s.handleGetCube)
			r.Get("/cubes/{id}/cards", s.handleGetCubeCards)
			r.Get("/cubes/{id}/combos", s.handleListCombos)

			// Per-card detail is cube-scoped: it names the decks that play
			// the card, which is playgroup data even though the card itself
			// is not.
			r.Get("/cards/{slug}", s.handleGetCard)

			// Decklists
			r.Get("/decklists", s.handleListDecklists)
			r.Get("/decklists/{id}", s.handleGetDecklist)
			r.Post("/decklists", s.handleCreateDecklist)
			r.Patch("/decklists/{id}", s.handlePatchDecklist)
			r.Patch("/decklists/{id}/record", s.handlePatchDecklistRecord)
			r.Delete("/decklists/{id}", s.handleDeleteDecklist)
			r.Post("/decklists/infer-colors", s.handleInferColors)

			// Analytics — every stat here is derived from decks and games.
			r.Get("/analytics/overview", s.handleAnalyticsOverview)
			r.Get("/analytics/colors", s.handleAnalyticsColors)
			r.Get("/analytics/color-trend", s.handleAnalyticsColorTrend)
			r.Get("/analytics/cards", s.handleAnalyticsCards)
			r.Get("/analytics/pairs", s.handleAnalyticsPairs)
		})

		// --- Admin ---
		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)

			r.Delete("/users/{id}", s.handleDeleteUser)
			r.Post("/admin/users", s.handleCreateUser)

			r.Post("/admin/cubes", s.handleCreateCube)
			r.Patch("/admin/cubes/{id}", s.handlePatchCube)
			r.Delete("/admin/cubes/{id}", s.handleDeleteCube)
			r.Post("/admin/cubes/{id}/sync", s.handleSyncCube)
			r.Get("/admin/cubes/{id}/sync-status", s.handleCubeSyncStatus)

			r.Post("/admin/cubes/{id}/combos", s.handleCreateCombo)
			r.Patch("/admin/combos/{id}", s.handlePatchCombo)
			r.Delete("/admin/combos/{id}", s.handleDeleteCombo)

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

// apiError carries the response a failed helper wants written, so validation
// shared by several handlers can report its own status instead of collapsing to
// a single one at the call site. Write it with writeAPIErr.
type apiError struct {
	status int
	msg    string
}

func (e apiError) Error() string { return e.msg }

func writeAPIErr(w http.ResponseWriter, err error) {
	var ae apiError
	if errors.As(err, &ae) {
		writeErr(w, ae.status, ae.msg)
		return
	}
	writeErr(w, http.StatusInternalServerError, "internal error")
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
