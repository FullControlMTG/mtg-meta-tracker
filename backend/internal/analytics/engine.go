// Package analytics recomputes precomputed meta snapshots from decklists.
package analytics

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/config"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// Engine runs the recompute pipeline and triggers frontend revalidation.
type Engine struct {
	store *store.Store
	cfg   config.Config
	http  *http.Client
}

func NewEngine(s *store.Store, cfg config.Config) *Engine {
	return &Engine{store: s, cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
}

// Recompute loads a cube's decklists, computes all stat snapshots, writes a new
// analytics run, promotes it to current, and revalidates affected pages.
func (e *Engine) Recompute(ctx context.Context, cubeID uuid.UUID, trigger string) error {
	runID, err := e.store.CreateAnalyticsRun(ctx, cubeID, trigger)
	if err != nil {
		return err
	}

	// Deck colors are inferred at save time, so a deck saved under an older rule
	// carries an older answer. Re-derive them first and every run aggregates the
	// colors the current rule would give.
	changed, err := e.store.RecomputeDeckColors(ctx, cubeID)
	if err != nil {
		_ = e.store.SetAnalyticsRunFailed(ctx, runID)
		return err
	}
	if changed > 0 {
		log.Printf("analytics: cube %s: recomputed colors for %d deck(s)", cubeID, changed)
	}

	decks, err := e.store.LoadDecksForAnalytics(ctx, cubeID)
	if err != nil {
		_ = e.store.SetAnalyticsRunFailed(ctx, runID)
		return err
	}
	if len(decks) == 0 {
		if err := e.store.FinalizeEmptyRun(ctx, runID, cubeID); err != nil {
			return err
		}
		e.revalidate(ctx, cubeID)
		return nil
	}

	cards, err := e.store.LoadDeckCardsForAnalytics(ctx, cubeID)
	if err != nil {
		_ = e.store.SetAnalyticsRunFailed(ctx, runID)
		return err
	}

	results := aggregate(decks, cards)

	if err := e.store.FinalizeAnalyticsRun(ctx, runID, cubeID, results); err != nil {
		_ = e.store.SetAnalyticsRunFailed(ctx, runID)
		return err
	}

	log.Printf("analytics: cube %s run %s ok (%d decks, %d games)",
		cubeID, runID, results.DecksIncluded, results.GamesIncluded)
	e.revalidate(ctx, cubeID)
	return nil
}
