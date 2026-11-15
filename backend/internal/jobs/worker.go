package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

type Handler func(ctx context.Context, payload json.RawMessage) error

// MaxAttempts caps how many times a job runs before it stays failed. Failures are
// mostly transient (Scryfall down, a deadlock), so a couple of retries heal them;
// a job that is simply broken stops within minutes.
const MaxAttempts = 3

// retryDelay backs off a failed job: 1m after its first attempt, then 2m.
func retryDelay(attempts int) time.Duration {
	return time.Minute << (attempts - 1)
}

// Queue is the slice of *store.Store the worker drives, so tests can fake it.
type Queue interface {
	ClaimNextJob(ctx context.Context) (*domain.Job, error)
	CompleteJob(ctx context.Context, id uuid.UUID) error
	FailJob(ctx context.Context, id uuid.UUID, msg string) error
	RetryJob(ctx context.Context, id uuid.UUID, msg string, delay time.Duration) error
}

type Worker struct {
	store    Queue
	handlers map[string]Handler
	interval time.Duration

	// mu is held for a whole drain, so Pause can wait out the job in flight.
	mu     sync.Mutex
	paused atomic.Bool
}

func NewWorker(s Queue, pollInterval time.Duration) *Worker {
	return &Worker{store: s, handlers: map[string]Handler{}, interval: pollInterval}
}

func (w *Worker) Register(jobType string, h Handler) { w.handlers[jobType] = h }

// Pause stops the worker claiming jobs and returns once the job in flight, if
// any, has finished. A database restore needs the worker off the tables it drops.
func (w *Worker) Pause() {
	w.paused.Store(true)
	w.mu.Lock()
	w.mu.Unlock()
}

func (w *Worker) Resume() { w.paused.Store(false) }

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
	w.mu.Lock()
	defer w.mu.Unlock()
	for !w.paused.Load() {
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
			w.fail(ctx, job, err)
			continue
		}
		_ = w.store.CompleteJob(ctx, job.ID)
	}
}

// fail records a handler error, re-queueing the job while it has attempts left.
// An unknown type is not retried: no attempt would find a handler either.
func (w *Worker) fail(ctx context.Context, job *domain.Job, err error) {
	if job.Attempts < MaxAttempts {
		delay := retryDelay(job.Attempts)
		log.Printf("jobs: %s failed (attempt %d/%d), retrying in %s: %v",
			job.Type, job.Attempts, MaxAttempts, delay, err)
		_ = w.store.RetryJob(ctx, job.ID, err.Error(), delay)
		return
	}
	log.Printf("jobs: %s failed (attempt %d/%d), giving up: %v",
		job.Type, job.Attempts, MaxAttempts, err)
	_ = w.store.FailJob(ctx, job.ID, err.Error())
}
