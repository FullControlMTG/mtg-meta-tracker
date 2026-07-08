// Package revalidate best-effort pings the Next.js on-demand revalidation
// webhook so ISR-cached static pages re-render after a backend change.
package revalidate

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

var defaultClient = &http.Client{Timeout: 10 * time.Second}

// Post fires the revalidation webhook for the given paths. It never fails the
// caller: on any error it logs and returns. A nil client falls back to a shared
// default. A blank url (webhook not configured) or empty paths is a no-op.
func Post(ctx context.Context, client *http.Client, url, secret string, paths []string) {
	if url == "" || len(paths) == 0 {
		return
	}
	if client == nil {
		client = defaultClient
	}
	body, _ := json.Marshal(map[string][]string{"paths": paths})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("revalidate build: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-revalidate-secret", secret)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("revalidate: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		log.Printf("revalidate status %d", resp.StatusCode)
	}
}
