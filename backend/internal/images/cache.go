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
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"
)

// allowedHost restricts downloads to the Scryfall image CDN. Source URLs come
// from our own DB, but validating the host keeps this from becoming an open
// proxy (SSRF guard).
const allowedHost = "cards.scryfall.io"

const (
	// maxConcurrentDownloads caps simultaneous cold-cache downloads from Scryfall.
	// A ~540-card cube page would otherwise fire hundreds of concurrent GETs and
	// get rate-limited; 6 mirrors the browser's ~6-connections-per-host limit and
	// keeps sustained throughput well under Scryfall's guidance.
	maxConcurrentDownloads = 6
	// downloadAttempts is the number of tries per image before giving up.
	downloadAttempts = 3
	// maxRetryAfter caps how long we honor a Retry-After header, so one hostile
	// header can't stall a download slot indefinitely.
	maxRetryAfter = 10 * time.Second
)

type Cache struct {
	dir       string
	userAgent string
	http      *http.Client
	group     singleflight.Group
	// sem bounds concurrent downloads (buffered channel used as a semaphore).
	sem chan struct{}
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
		sem:       make(chan struct{}, maxConcurrentDownloads),
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
		// Acquire a download slot only here — after the re-check — so waiters
		// collapsed onto this key by singleflight don't each burn a slot; only
		// the goroutine that actually downloads holds one. Held through retry
		// backoff so a full semaphore also throttles retry throughput.
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
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
	// URL/host validation is permanent — never retry it.
	u, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("parse source url: %w", err)
	}
	if u.Scheme != "https" || u.Host != allowedHost {
		return fmt.Errorf("disallowed image source %q", sourceURL)
	}

	var lastErr error
	for attempt := 0; attempt < downloadAttempts; attempt++ {
		if attempt > 0 {
			// Retry: back off before trying again. Sleep is context-aware so a
			// cancelled request stops retrying and frees its semaphore slot.
			if err := sleep(ctx, backoff(attempt, lastErr)); err != nil {
				return err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.userAgent)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err // transport error — retryable
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = &httpError{status: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
			_ = resp.Body.Close()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			// Other non-200 (e.g. 404 stale source) is permanent — don't retry.
			_ = resp.Body.Close()
			return fmt.Errorf("scryfall image %s: status %d", sourceURL, resp.StatusCode)
		}

		if err := c.store(resp.Body, dest); err != nil {
			_ = resp.Body.Close()
			return err
		}
		_ = resp.Body.Close()
		return nil
	}
	return fmt.Errorf("scryfall image %s: %w", sourceURL, lastErr)
}

// store writes body to a temp file then renames it into place so readers never
// see a partial file.
func (c *Cache) store(body io.Reader, dest string) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dir, "dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, body); err != nil {
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

// httpError carries a retryable Scryfall status plus any Retry-After hint.
type httpError struct {
	status     int
	retryAfter time.Duration
}

func (e *httpError) Error() string { return fmt.Sprintf("status %d", e.status) }

// backoff returns how long to wait before the given attempt (1-indexed for
// retries). It honors a Retry-After hint from the last error when present,
// otherwise a linear ramp with jitter so concurrent retriers de-synchronize.
func backoff(attempt int, lastErr error) time.Duration {
	if he, ok := lastErr.(*httpError); ok && he.retryAfter > 0 {
		return he.retryAfter
	}
	base := time.Duration(attempt) * 500 * time.Millisecond
	return base + jitter(base)
}

// jitter returns a small +/-20% perturbation of d.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	// Deterministic source (time-based) is fine here; we only need retriers to
	// spread out, not cryptographic randomness.
	n := time.Now().UnixNano()
	frac := float64(n%1000)/1000*0.4 - 0.2 // [-0.2, 0.2)
	return time.Duration(float64(d) * frac)
}

// parseRetryAfter reads a Retry-After header value (delta-seconds form),
// clamped to maxRetryAfter. Returns 0 when absent or unparseable.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
