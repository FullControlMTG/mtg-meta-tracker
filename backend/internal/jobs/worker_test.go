package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// fakeQueue hands out its jobs in order, then reports an empty queue, and records
// how each one ended.
type fakeQueue struct {
	jobs     []*domain.Job
	claimErr error
	done     []uuid.UUID
	failed   map[uuid.UUID]string
	retried  map[uuid.UUID]time.Duration
}

func (q *fakeQueue) ClaimNextJob(context.Context) (*domain.Job, error) {
	if q.claimErr != nil {
		return nil, q.claimErr
	}
	if len(q.jobs) == 0 {
		return nil, store.ErrNotFound
	}
	j := q.jobs[0]
	q.jobs = q.jobs[1:]
	return j, nil
}

func (q *fakeQueue) CompleteJob(_ context.Context, id uuid.UUID) error {
	q.done = append(q.done, id)
	return nil
}

func (q *fakeQueue) FailJob(_ context.Context, id uuid.UUID, msg string) error {
	if q.failed == nil {
		q.failed = map[uuid.UUID]string{}
	}
	q.failed[id] = msg
	return nil
}

func (q *fakeQueue) RetryJob(_ context.Context, id uuid.UUID, _ string, delay time.Duration) error {
	if q.retried == nil {
		q.retried = map[uuid.UUID]time.Duration{}
	}
	q.retried[id] = delay
	return nil
}

// A freshly claimed job: ClaimNextJob has already counted this attempt.
func job(typ, payload string) *domain.Job {
	return &domain.Job{ID: uuid.New(), Type: typ, Payload: json.RawMessage(payload), Attempts: 1}
}

func TestDrainRunsEveryJobToCompletion(t *testing.T) {
	ok, bad, unknown := job("ok", `{"n":1}`), job("bad", `{}`), job("nobody", `{}`)
	q := &fakeQueue{jobs: []*domain.Job{ok, bad, unknown}}
	w := NewWorker(q, 0)

	var got []string
	w.Register("ok", func(_ context.Context, p json.RawMessage) error {
		got = append(got, string(p))
		return nil
	})
	w.Register("bad", func(context.Context, json.RawMessage) error { return errors.New("boom") })

	w.drain(context.Background())

	if !reflect.DeepEqual(got, []string{`{"n":1}`}) {
		t.Fatalf("ok handler saw %v", got)
	}
	if !reflect.DeepEqual(q.done, []uuid.UUID{ok.ID}) {
		t.Fatalf("completed %v, want only the ok job", q.done)
	}
	// A first failure goes back in the queue rather than failing for good.
	if d, ok := q.retried[bad.ID]; !ok || d != time.Minute {
		t.Errorf("bad job retried=%v delay=%s, want a retry after 1m", ok, d)
	}
	// A job nobody handles must fail rather than sit claimed as running forever.
	if q.failed[unknown.ID] != "no handler for type nobody" {
		t.Errorf("unknown job failed with %q", q.failed[unknown.ID])
	}
	if len(q.jobs) != 0 {
		t.Fatalf("%d job(s) left unclaimed", len(q.jobs))
	}
}

func TestDrainStopsOnClaimError(t *testing.T) {
	q := &fakeQueue{claimErr: errors.New("db down")}
	w := NewWorker(q, 0)
	// Returns instead of spinning on a broken database.
	w.drain(context.Background())
	if len(q.done) != 0 || len(q.failed) != 0 {
		t.Fatalf("done %v failed %v, want nothing touched", q.done, q.failed)
	}
}

func TestFailedJobRetriesUntilTheAttemptCap(t *testing.T) {
	second, last := job("bad", `{}`), job("bad", `{}`)
	second.Attempts, last.Attempts = 2, MaxAttempts
	q := &fakeQueue{jobs: []*domain.Job{second, last}}
	w := NewWorker(q, 0)
	w.Register("bad", func(context.Context, json.RawMessage) error { return errors.New("boom") })

	w.drain(context.Background())

	if d := q.retried[second.ID]; d != 2*time.Minute {
		t.Errorf("second attempt retried after %s, want 2m", d)
	}
	if _, ok := q.retried[last.ID]; ok {
		t.Errorf("job on its last attempt was retried")
	}
	if q.failed[last.ID] != "boom" {
		t.Errorf("last attempt failed with %q, want the handler's error", q.failed[last.ID])
	}
}
