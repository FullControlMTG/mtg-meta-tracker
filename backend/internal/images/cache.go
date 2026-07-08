// Package images is an on-disk cache/proxy for card images. It downloads bytes
// from Scryfall lazily on a cache miss and serves them from a local directory
// thereafter, so the frontend never hotlinks the Scryfall CDN and repeat views
// survive Scryfall being slow or down.
package images

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/singleflight"
)

// allowedHost restricts downloads to the Scryfall image CDN. Source URLs come
// from our own DB, but validating the host keeps this from becoming an open
// proxy (SSRF guard).
const allowedHost = "cards.scryfall.io"

type Cache struct {
	dir       string
	userAgent string
	http      *http.Client
	group     singleflight.Group
}

// New creates a cache rooted at dir. An empty dir falls back to a temp subdir
// (ephemeral across restarts). The directory is created on first write.
func New(dir, userAgent string) *Cache {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "mtg-image-cache")
	}
	return &Cache{
		dir:       dir,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch returns the local filesystem path for key, downloading from sourceURL on
// a miss. Concurrent misses for the same key are collapsed via singleflight.
func (c *Cache) Fetch(ctx context.Context, key, sourceURL string) (string, error) {
	path := c.pathFor(key)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		// Re-check inside the singleflight in case a sibling just finished.
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		if err := c.download(ctx, sourceURL, path); err != nil {
			return nil, err
		}
		return path, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *Cache) pathFor(key string) string {
	// key is <scryfall_id>-<variant>: uuid + a fixed word, already filesystem-safe.
	return filepath.Join(c.dir, key+".img")
}

func (c *Cache) download(ctx context.Context, sourceURL, dest string) error {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("parse source url: %w", err)
	}
	if u.Scheme != "https" || u.Host != allowedHost {
		return fmt.Errorf("disallowed image source %q", sourceURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scryfall image %s: status %d", sourceURL, resp.StatusCode)
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	// Write to a temp file then rename so readers never see a partial file.
	tmp, err := os.CreateTemp(c.dir, "dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
