package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// revalidate best-effort pings the Next.js on-demand revalidation webhook so the
// static pages reflecting this cube's analytics re-render. Never fails the run.
func (e *Engine) revalidate(ctx context.Context, cubeID uuid.UUID) {
	if e.cfg.RevalidateURL == "" {
		return
	}
	paths := []string{"/", "/analytics", "/decklists", "/cubes/" + cubeID.String()}
	// Also refresh each affected deck's detail page (long-cached at revalidate=3600),
	// so record/deck edits surface promptly.
	if ids, err := e.store.ListCubeDecklistIDs(ctx, cubeID); err == nil {
		for _, id := range ids {
			paths = append(paths, "/decklists/"+id.String())
		}
	}
	body, _ := json.Marshal(map[string][]string{"paths": paths})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.RevalidateURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("analytics: revalidate build: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-revalidate-secret", e.cfg.RevalidateSecret)

	resp, err := e.http.Do(req)
	if err != nil {
		log.Printf("analytics: revalidate: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		log.Printf("analytics: revalidate status %d", resp.StatusCode)
	}
}
