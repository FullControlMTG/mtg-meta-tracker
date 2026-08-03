package analytics

import (
	"context"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/revalidate"
)

// revalidate best-effort pings the Next.js on-demand revalidation webhook so the
// static pages reflecting this cube's analytics re-render. Never fails the run.
func (e *Engine) revalidate(ctx context.Context, cubeID uuid.UUID) {
	// The cube overview and its pool + deck list. Per-card pages (/cube/<id>/cards/<slug>)
	// are deliberately not revalidated — hundreds per cube, each with its own short ISR.
	cube := "/cube/" + cubeID.String()
	paths := []string{"/", cube, cube + "/cards", cube + "/decks"}
	// Also refresh each affected deck's detail page (long-cached at revalidate=3600).
	if ids, err := e.store.ListCubeDecklistIDs(ctx, cubeID); err == nil {
		for _, id := range ids {
			paths = append(paths, cube+"/decks/"+id.String())
		}
	}
	revalidate.Post(ctx, e.http, e.cfg.RevalidateURL, e.cfg.RevalidateSecret, paths)
}
