package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "SESSION_COOKIE_SECURE", "SESSION_TTL_HOURS",
		"SCRYFALL_MIN_INTERVAL_MS", "SYNC_INTERVAL_MINUTES", "APP_TIMEZONE"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.DatabaseURL != "postgres://mtg:mtg@localhost:5432/mtg_meta?sslmode=disable" {
		t.Errorf("DatabaseURL = %q; the default must reach the make db-up database", c.DatabaseURL)
	}
	if !c.SessionCookieSecure {
		t.Error("SessionCookieSecure defaults false; an unconfigured prod would drop Secure")
	}
	if c.SessionTTL != 720*time.Hour {
		t.Errorf("SessionTTL = %v", c.SessionTTL)
	}
	if c.ScryfallMinInterval != 100*time.Millisecond {
		t.Errorf("ScryfallMinInterval = %v", c.ScryfallMinInterval)
	}
	if c.SyncInterval != 360*time.Minute {
		t.Errorf("SyncInterval = %v", c.SyncInterval)
	}
	if c.Timezone.String() != "America/Los_Angeles" {
		t.Errorf("Timezone = %v", c.Timezone)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("SESSION_TTL_HOURS", "2")
	t.Setenv("SYNC_INTERVAL_MINUTES", "0")
	t.Setenv("APP_TIMEZONE", "Europe/Berlin")
	c := Load()
	if c.HTTPAddr != ":9090" || c.SessionCookieSecure || c.SessionTTL != 2*time.Hour ||
		c.SyncInterval != 0 || c.Timezone.String() != "Europe/Berlin" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestEnvIntFallsBackOnGarbage(t *testing.T) {
	t.Setenv("X_INT", "twelve")
	if got := envInt("X_INT", 7); got != 7 {
		t.Fatalf("envInt = %d, want the default 7", got)
	}
	t.Setenv("X_INT", "12")
	if got := envInt("X_INT", 7); got != 12 {
		t.Fatalf("envInt = %d, want 12", got)
	}
}

// A typo in a security flag must not quietly turn it off.
func TestEnvBoolFallsBackOnGarbage(t *testing.T) {
	cases := []struct {
		val  string
		def  bool
		want bool
	}{
		{"", true, true},
		{"false", true, false},
		{"0", true, false},
		{"TRUE", false, true},
		{"flase", true, true},
		{"yes", true, true},
	}
	for _, tc := range cases {
		t.Setenv("X_BOOL", tc.val)
		if got := envBool("X_BOOL", tc.def); got != tc.want {
			t.Errorf("envBool(%q, %v) = %v, want %v", tc.val, tc.def, got, tc.want)
		}
	}
}

func TestLocationFallsBackToUTC(t *testing.T) {
	if loc := location("Mars/Olympus_Mons"); loc != time.UTC {
		t.Fatalf("unknown zone gave %v, want UTC", loc)
	}
	// Resolves from the embedded tzdata, which is all a bare container has.
	if loc := location("Asia/Tokyo"); loc.String() != "Asia/Tokyo" {
		t.Fatalf("location = %v", loc)
	}
}
