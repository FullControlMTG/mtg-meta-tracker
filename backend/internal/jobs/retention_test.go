package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePrune struct {
	jobsBefore, runsBefore []time.Time
	jobsErr                error
}

func (f *fakePrune) PruneJobs(_ context.Context, before time.Time) (int64, error) {
	f.jobsBefore = append(f.jobsBefore, before)
	return 2, f.jobsErr
}

func (f *fakePrune) PruneAnalyticsRuns(_ context.Context, before time.Time) (int64, error) {
	f.runsBefore = append(f.runsBefore, before)
	return 1, nil
}

func TestPrunerCutsOffAtKeep(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	f := &fakePrune{}
	p := NewPruner(f, 30*24*time.Hour)
	p.now = func() time.Time { return now }

	if err := p.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if len(f.jobsBefore) != 1 || !f.jobsBefore[0].Equal(want) {
		t.Errorf("jobs pruned before %v, want %v", f.jobsBefore, want)
	}
	if len(f.runsBefore) != 1 || !f.runsBefore[0].Equal(want) {
		t.Errorf("runs pruned before %v, want %v", f.runsBefore, want)
	}
}

// Zero keep must not read as "keep nothing" and empty both tables.
func TestPrunerDisabledByZeroKeep(t *testing.T) {
	f := &fakePrune{}
	if err := NewPruner(f, 0).Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(f.jobsBefore)+len(f.runsBefore) != 0 {
		t.Fatalf("pruned with keep=0: jobs %v runs %v", f.jobsBefore, f.runsBefore)
	}
}

// The error reaches the worker, which retries the job.
func TestPrunerReturnsStoreError(t *testing.T) {
	f := &fakePrune{jobsErr: errors.New("db down")}
	if err := NewPruner(f, time.Hour).Handle(context.Background(), nil); err == nil {
		t.Fatal("want the store error")
	}
	if len(f.runsBefore) != 0 {
		t.Errorf("pruned runs after the jobs prune failed")
	}
}
