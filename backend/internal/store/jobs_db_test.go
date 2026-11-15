package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// insertJob writes a jobs row directly, so a test can stage any queue state.
func insertJob(t *testing.T, st *Store, status string, attempts int, key *string, finished *time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := st.pool.QueryRow(context.Background(), `
		INSERT INTO jobs (type, status, attempts, dedup_key, finished_at)
		VALUES ('sync_cube',$1,$2,$3,$4) RETURNING id`,
		status, attempts, key, finished).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func jobStatus(t *testing.T, st *Store, id uuid.UUID) string {
	t.Helper()
	var s string
	err := st.pool.QueryRow(context.Background(), `SELECT status FROM jobs WHERE id=$1`, id).Scan(&s)
	if err != nil {
		return "gone"
	}
	return s
}

func key(s string) *string { return &s }

func TestRecoverJobs(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)

	interrupted := insertJob(t, st, "running", 1, key("a"), nil)
	exhausted := insertJob(t, st, "running", 3, nil, nil)
	superseded := insertJob(t, st, "running", 1, key("b"), nil)
	newer := insertJob(t, st, "pending", 0, key("b"), nil)

	requeued, failed, err := st.RecoverJobs(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 1 || failed != 2 {
		t.Errorf("requeued %d failed %d, want 1 and 2", requeued, failed)
	}
	for id, want := range map[uuid.UUID]string{
		interrupted: "pending", exhausted: "failed", superseded: "failed", newer: "pending",
	} {
		if got := jobStatus(t, st, id); got != want {
			t.Errorf("job %s is %s, want %s", id, got, want)
		}
	}
}

func TestRetryJob(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)

	alone := insertJob(t, st, "running", 1, key("a"), nil)
	if err := st.RetryJob(ctx, alone, "boom", time.Minute); err != nil {
		t.Fatal(err)
	}
	var due bool
	if err := st.pool.QueryRow(ctx,
		`SELECT scheduled_at > now() FROM jobs WHERE id=$1`, alone).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if got := jobStatus(t, st, alone); got != "pending" || !due {
		t.Errorf("retried job is %s (scheduled later: %v), want pending after the delay", got, due)
	}

	// A pending job with the same key already covers the work.
	dup := insertJob(t, st, "running", 1, key("b"), nil)
	insertJob(t, st, "pending", 0, key("b"), nil)
	if err := st.RetryJob(ctx, dup, "boom", time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := jobStatus(t, st, dup); got != "failed" {
		t.Errorf("superseded job is %s, want failed", got)
	}
}

func TestPruneJobs(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	old, recent := time.Now().Add(-48*time.Hour), time.Now()

	oldDone := insertJob(t, st, "done", 1, nil, &old)
	oldFailed := insertJob(t, st, "failed", 3, nil, &old)
	newDone := insertJob(t, st, "done", 1, nil, &recent)
	pending := insertJob(t, st, "pending", 0, nil, nil)

	n, err := st.PruneJobs(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("pruned %d, want 2", n)
	}
	for id, want := range map[uuid.UUID]string{
		oldDone: "gone", oldFailed: "gone", newDone: "done", pending: "pending",
	} {
		if got := jobStatus(t, st, id); got != want {
			t.Errorf("job %s is %s, want %s", id, got, want)
		}
	}
}

func TestPruneAnalyticsRunsKeepsCurrentAndCascades(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	owner := mustUser(t, st, "owner", "user")
	cube := &domain.Cube{Name: "cube", OwnerID: &owner.ID}
	if err := st.CreateCube(ctx, cube); err != nil {
		t.Fatal(err)
	}

	run := func(current bool, age time.Duration) uuid.UUID {
		var id uuid.UUID
		if err := st.pool.QueryRow(ctx, `
			INSERT INTO analytics_runs (cube_id, trigger, status, is_current, started_at)
			VALUES ($1,'manual','ok',$2,$3) RETURNING id`,
			cube.ID, current, time.Now().Add(-age)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO color_stats (run_id, facet, facet_key, deck_count)
			VALUES ($1,'single_color',1,1)`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	current := run(true, 72*time.Hour)
	stale := run(false, 48*time.Hour)
	fresh := run(false, time.Hour)

	n, err := st.PruneAnalyticsRuns(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d runs, want 1", n)
	}
	for id, want := range map[uuid.UUID]int{current: 1, stale: 0, fresh: 1} {
		var runs, stats int
		if err := st.pool.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM analytics_runs WHERE id=$1),
			       (SELECT count(*) FROM color_stats WHERE run_id=$1)`, id).Scan(&runs, &stats); err != nil {
			t.Fatal(err)
		}
		if runs != want || stats != want {
			t.Errorf("run %s: %d run row(s), %d stat row(s), want %d of each", id, runs, stats, want)
		}
	}
}
