package jobs

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// PruneJobType is the job that applies the retention policy. The scheduler
// enqueues it, so it runs on the worker like any other job: serialized with
// recomputes, and paused along with them during a database restore.
const PruneJobType = "prune"

// PruneStore is the slice of *store.Store retention drives, so tests can fake it.
type PruneStore interface {
	PruneJobs(ctx context.Context, before time.Time) (int64, error)
	PruneAnalyticsRuns(ctx context.Context, before time.Time) (int64, error)
}

// Pruner deletes finished jobs and superseded analytics runs older than keep.
type Pruner struct {
	store PruneStore
	keep  time.Duration
	now   func() time.Time
}

func NewPruner(s PruneStore, keep time.Duration) *Pruner {
	return &Pruner{store: s, keep: keep, now: time.Now}
}

// Handle is the prune job's Handler. A keep of zero or less disables pruning
// rather than deleting everything up to now.
func (p *Pruner) Handle(ctx context.Context, _ json.RawMessage) error {
	if p.keep <= 0 {
		return nil
	}
	before := p.now().Add(-p.keep)
	jobs, err := p.store.PruneJobs(ctx, before)
	if err != nil {
		return err
	}
	runs, err := p.store.PruneAnalyticsRuns(ctx, before)
	if err != nil {
		return err
	}
	if jobs > 0 || runs > 0 {
		log.Printf("retention: pruned %d job(s) and %d analytics run(s) older than %s",
			jobs, runs, before.Format(time.RFC3339))
	}
	return nil
}
