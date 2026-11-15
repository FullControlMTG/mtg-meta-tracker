package jobs

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

type enqueued struct {
	jobType string
	payload any
	key     string
}

type fakeCubes struct {
	cubes   []domain.Cube
	listErr error
	failFor uuid.UUID
	got     []enqueued
}

func (f *fakeCubes) ListCubes(context.Context) ([]domain.Cube, error) {
	return f.cubes, f.listErr
}

func (f *fakeCubes) EnqueueJob(_ context.Context, jobType string, payload any, key string) error {
	if p, ok := payload.(map[string]string); ok && p["cube_id"] == f.failFor.String() {
		return errors.New("enqueue failed")
	}
	f.got = append(f.got, enqueued{jobType, payload, key})
	return nil
}

func ptr(s string) *string { return &s }

func TestTickEnqueuesOnlyCubesWithAList(t *testing.T) {
	listed, failing := uuid.New(), uuid.New()
	f := &fakeCubes{
		failFor: failing,
		cubes: []domain.Cube{
			{ID: uuid.New()},                      // never given a list
			{ID: uuid.New(), CardList: ptr("")},   // list cleared
			{ID: failing, CardList: ptr("1 Opt")}, // enqueue errors; the rest still run
			{ID: listed, CardList: ptr("1 Bolt")},
		},
	}
	NewScheduler(f, time.Hour).tick(context.Background())

	want := []enqueued{{
		jobType: "sync_cube",
		payload: map[string]string{"cube_id": listed.String()},
		// The dedup key is what stops a slow sync being queued twice.
		key: "sync_cube:" + listed.String(),
	}}
	if !reflect.DeepEqual(f.got, want) {
		t.Fatalf("enqueued %+v, want %+v", f.got, want)
	}
}

func TestTickSurvivesListError(t *testing.T) {
	f := &fakeCubes{listErr: errors.New("db down")}
	NewScheduler(f, time.Hour).tick(context.Background())
	if len(f.got) != 0 {
		t.Fatalf("enqueued %v after a list error", f.got)
	}
}

func TestRunReturns(t *testing.T) {
	f := &fakeCubes{cubes: []domain.Cube{{ID: uuid.New(), CardList: ptr("1 Bolt")}}}

	// A zero interval disables the scheduler outright.
	NewScheduler(f, 0).Run(context.Background())

	// A cancelled context stops it before the startup kick.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewScheduler(f, time.Hour).Run(ctx)

	if len(f.got) != 0 {
		t.Fatalf("enqueued %v; neither run should have ticked", f.got)
	}
}
