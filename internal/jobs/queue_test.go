package jobs

import (
	"context"
	"testing"

	"velopim/internal/dbtest"
)

// TestEnqueueFetchIsUniqueWhilePending runs against River's real tables:
// a waiting fetch is not queued twice, but once it has finished the source
// can be fetched again.
func TestEnqueueFetchIsUniqueWhilePending(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	queue, err := New(pool, Config{Fetcher: &Fetcher{Pool: pool, RawDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	count := func(where string) string {
		return one(t, pool, `SELECT count(*)::text FROM river_job WHERE kind = 'fetch_source' AND `+where)
	}

	// Not started: jobs stay waiting.
	for range 3 {
		if err := queue.EnqueueFetch(ctx, "hartje"); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.EnqueueFetch(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if got := count(`args->>'source_id' = 'hartje'`); got != "1" {
		t.Errorf("waiting fetch jobs for one source = %s, want 1", got)
	}
	if got := count(`true`); got != "2" {
		t.Errorf("fetch jobs for two sources = %s, want 2", got)
	}
	if got := one(t, pool, `SELECT max_attempts::text FROM river_job WHERE kind = 'fetch_source' LIMIT 1`); got != "1" {
		t.Errorf("max attempts = %s, want 1 (no automatic retry)", got)
	}

	// The job finishes: the next fetch of the same source is accepted.
	if _, err := pool.Exec(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = 'fetch_source' AND args->>'source_id' = 'hartje'`); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueFetch(ctx, "hartje"); err != nil {
		t.Fatal(err)
	}
	if got := count(`args->>'source_id' = 'hartje'`); got != "2" {
		t.Errorf("fetch jobs after the first finished = %s, want 2", got)
	}
	if got := count(`args->>'source_id' = 'hartje' AND state = 'available'`); got != "1" {
		t.Errorf("waiting fetch jobs = %s, want 1", got)
	}
}
