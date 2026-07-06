package httpapi

import (
	"net/http"
	"time"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/auth"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// resolveCaller attaches a Caller (Public or Authenticated) built from the
// session cookie to every request context.
func (s *Server) resolveCaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := appctx.PublicCaller
		if ck, err := r.Cookie(s.cfg.SessionCookieName); err == nil && ck.Value != "" {
			if u, err := s.store.GetValidSessionUser(r.Context(), auth.HashToken(ck.Value)); err == nil {
				caller = appctx.Caller{
					Kind:   appctx.Authenticated,
					UserID: u.ID,
					Role:   roleFromString(u.Role),
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(appctx.With(r.Context(), caller)))
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !appctx.From(r.Context()).IsAuthenticated() {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !appctx.From(r.Context()).IsAdmin() {
			writeErr(w, http.StatusForbidden, "admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func roleFromString(role string) appctx.Role {
	if role == domain.RoleAdmin {
		return appctx.RoleAdmin
	}
	return appctx.RoleUser
}

// setSessionCookie / clearSessionCookie manage the session cookie.
func (s *Server) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
