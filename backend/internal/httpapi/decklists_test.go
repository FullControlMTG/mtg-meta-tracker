package httpapi

import (
	"testing"
	"time"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/config"
)

// A server with only the field parsePlayedAt reads: the playgroup's timezone.
func tzServer(t *testing.T, zone string) *Server {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("load %s: %v", zone, err)
	}
	return &Server{cfg: config.Config{Timezone: loc}}
}

func TestParsePlayedAt(t *testing.T) {
	s := tzServer(t, "America/Los_Angeles")
	now := time.Now().In(s.cfg.Timezone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{in: "", want: today},
		{in: "   ", want: today},
		{in: "2026-07-24", want: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)},
		// Served as RFC3339, so a client handing back what it was given still works —
		// and the zone is dropped rather than allowed to shift the calendar day.
		{in: "2026-07-24T00:00:00Z", want: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)},
		{in: "2026-07-24T23:30:00-07:00", want: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)},
		{in: "24/07/2026", wantErr: true},
		{in: "yesterday", wantErr: true},
	}
	for _, tc := range cases {
		got, err := s.parsePlayedAt(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parsePlayedAt(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePlayedAt(%q): %v", tc.in, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("parsePlayedAt(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// The whole point of the configured zone: the server runs in UTC, where the day has
// already turned over while it is still yesterday afternoon for the playgroup.
func TestTodayUsesConfiguredZone(t *testing.T) {
	la := tzServer(t, "America/Los_Angeles")
	utc := &Server{cfg: config.Config{Timezone: time.UTC}}

	laDay := la.today()
	utcDay := utc.today()
	if d := utcDay.Sub(laDay); d != 0 && d != 24*time.Hour {
		t.Errorf("LA today %v vs UTC today %v: want the same day or one behind", laDay, utcDay)
	}
	if h := time.Now().UTC().Hour(); h < 7 && laDay.Equal(utcDay) {
		t.Errorf("at %02d:00 UTC the LA day should still be the day before: %v == %v",
			h, laDay, utcDay)
	}
}
