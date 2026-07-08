package jobs

import (
	"context"
	"log"
	"time"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// Scheduler periodically enqueues sync_cube jobs for every cube that has a
// stored card_list. The sync_cube handler (via ingest.Syncer) does content-hash
// change detection, so an unchanged list is cheap (no Scryfall calls); this tick
// mainly self-heals cubes whose last resolve failed. The "sync_cube:<id>" dedup
// key means a still-pending sync is never duplicated.
type Scheduler struct {
	store    *store.Store
	interval time.Duration
}

func NewScheduler(s *store.Store, interval time.Duration) *Scheduler {
	return &Scheduler{store: s, interval: interval}
}

func (sc *Scheduler) Run(ctx context.Context) {
	if sc.interval <= 0 {
		log.Printf("scheduler: disabled (interval <= 0)")
		return
	}
	// Kick once shortly after startup so a fresh deploy syncs promptly.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
		sc.tick(ctx)
	}

	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sc.tick(ctx)
		}
	}
}

func (sc *Scheduler) tick(ctx context.Context) {
	cubes, err := sc.store.ListCubes(ctx)
	if err != nil {
		log.Printf("scheduler: list cubes: %v", err)
		return
	}
	n := 0
	for i := range cubes {
		c := &cubes[i]
		if c.CardList == nil || *c.CardList == "" {
			continue
		}
		if err := sc.store.EnqueueJob(ctx, "sync_cube",
			map[string]string{"cube_id": c.ID.String()}, "sync_cube:"+c.ID.String()); err != nil {
			log.Printf("scheduler: enqueue sync for cube %s: %v", c.ID, err)
			continue
		}
		n++
	}
	if n > 0 {
		log.Printf("scheduler: enqueued %d cube sync job(s)", n)
	}
}
