package images

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestStoreMode verifies cached image files are world-readable (0644), not the
// 0600 os.CreateTemp defaults to — otherwise a different reader UID can't serve them.
func TestStoreMode(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, "test-agent")

	dest := filepath.Join(dir, "xyz-normal.img")
	if err := c.store(strings.NewReader("imgbytes"), dest); err != nil {
		t.Fatalf("store: %v", err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %o, want 644", got)
	}
}

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

	// onProgress fires once per non-skipped item (the empty-URL item is skipped),
	// so two calls with monotonically rising done, ending at done=2, failed=1.
	var mu sync.Mutex
	var lastDone, lastFailed, calls int
	onProgress := func(done, failed int) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		lastDone, lastFailed = done, failed
	}

	if failed := c.Prefetch(context.Background(), items, onProgress); failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	if calls != 2 {
		t.Fatalf("onProgress calls = %d, want 2", calls)
	}
	if lastDone != 2 || lastFailed != 1 {
		t.Fatalf("final progress = %d done / %d failed, want 2/1", lastDone, lastFailed)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad-normal.img")); !os.IsNotExist(err) {
		t.Fatalf("disallowed item should not have been cached (stat err: %v)", err)
	}
}
