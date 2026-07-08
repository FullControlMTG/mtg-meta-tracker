package images

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPrefetch exercises the fan-out logic without hitting the network: an
// already-cached key is a hit (no download), an empty URL is skipped, and a
// disallowed-host URL fails the SSRF guard before any request — so exactly one
// item is reported failed and no file is written for it.
func TestPrefetch(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, "test-agent")

	const hitKey = "abc-normal"
	if err := os.WriteFile(filepath.Join(dir, hitKey+".img"), []byte("cached"), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	items := []PrefetchItem{
		// Cached hit: Fetch returns before any download, so the scryfall URL is never called.
		{Key: hitKey, URL: "https://cards.scryfall.io/normal/front/a/b.jpg"},
		// Empty URL: skipped entirely.
		{Key: "skip", URL: ""},
		// Disallowed host: rejected by the SSRF guard, counts as one failure.
		{Key: "bad-normal", URL: "https://evil.example.com/x.jpg"},
	}

	if failed := c.Prefetch(context.Background(), items); failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad-normal.img")); !os.IsNotExist(err) {
		t.Fatalf("disallowed item should not have been cached (stat err: %v)", err)
	}
}
