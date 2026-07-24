package config

import (
	"log"
	"os"
	"strconv"
	"time"

	// The zoneinfo database, compiled into the binary. The backend runs in a
	// scratch container with no /usr/share/zoneinfo, where LoadLocation would
	// otherwise fail and silently drop the app back to UTC.
	_ "time/tzdata"
)

type Config struct {
	DatabaseURL       string
	HTTPAddr          string
	SessionCookieName string
	SessionTTL        time.Duration

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	ScryfallUserAgent   string
	ScryfallMinInterval time.Duration

	// Directory for the on-disk card-image cache. Empty falls back to a temp
	// subdir (ephemeral); mount a volume here for a durable, shared cache.
	ImageCacheDir string

	// How often to poll Moxfield-backed cubes for list changes.
	SyncInterval time.Duration

	RevalidateURL    string
	RevalidateSecret string

	// The playgroup's timezone — the one "today" means when a deck is dated. It is
	// a local cube played in one place, so a single zone is the whole story; the
	// server's own clock is not, since it runs in UTC.
	Timezone *time.Location

	// First-admin bootstrap: applied only when the users table is empty.
	BootstrapAdminUsername string
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

func Load() Config {
	return Config{
		DatabaseURL:         env("DATABASE_URL", "postgres://mtg:mtg@localhost:5432/mtg_meta?sslmode=disable"),
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		SessionCookieName:   env("SESSION_COOKIE_NAME", "mtg_session"),
		SessionTTL:          time.Duration(envInt("SESSION_TTL_HOURS", 720)) * time.Hour,
		GoogleClientID:      env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:  env("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:   env("GOOGLE_REDIRECT_URL", ""),
		ScryfallUserAgent:   env("SCRYFALL_USER_AGENT", "mtg-meta-tracker/0.1"),
		ScryfallMinInterval: time.Duration(envInt("SCRYFALL_MIN_INTERVAL_MS", 100)) * time.Millisecond,
		ImageCacheDir:       env("IMAGE_CACHE_DIR", ""),
		SyncInterval:        time.Duration(envInt("SYNC_INTERVAL_MINUTES", 360)) * time.Minute,
		RevalidateURL:       env("REVALIDATE_URL", ""),
		RevalidateSecret:    env("REVALIDATE_SECRET", ""),
		Timezone:            location(env("APP_TIMEZONE", "America/Los_Angeles")),

		BootstrapAdminUsername: env("BOOTSTRAP_ADMIN_USERNAME", ""),
		BootstrapAdminEmail:    env("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword: env("BOOTSTRAP_ADMIN_PASSWORD", ""),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// location resolves an IANA zone name, falling back to UTC. A typo would otherwise
// date every deck in UTC without a word about it, so it is loud.
func location(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("config: unknown APP_TIMEZONE %q (%v); dating decks in UTC", name, err)
		return time.UTC
	}
	return loc
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
