package config

import (
	"os"
	"strconv"
	"time"
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

	AppBaseURL string // frontend origin, for invite links

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

		AppBaseURL: env("APP_BASE_URL", "http://localhost:3000"),

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

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
