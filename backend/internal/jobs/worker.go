package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

type Handler func(ctx context.Context, payload json.RawMessage) error

type Worker struct {
	store    *store.Store
	handlers map[string]Handler
	interval time.Duration
}

func NewWorker(s *store.Store, pollInterval time.Duration) *Worker {
	return &Worker{store: s, handlers: map[string]Handler{}, interval: pollInterval}
}

func (w *Worker) Register(jobType string, h Handler) { w.handlers[jobType] = h }

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	for {
		job, err := w.store.ClaimNextJob(ctx)
		if errors.Is(err, store.ErrNotFound) {
			return
		}
		if err != nil {
			log.Printf("jobs: claim: %v", err)
			return
		}
		h, ok := w.handlers[job.Type]
		if !ok {
			_ = w.store.FailJob(ctx, job.ID, "no handler for type "+job.Type)
			continue
		}
		if err := h(ctx, job.Payload); err != nil {
			log.Printf("jobs: %s failed: %v", job.Type, err)
			_ = w.store.FailJob(ctx, job.ID, err.Error())
			continue
		}
		_ = w.store.CompleteJob(ctx, job.ID)
	}
}
