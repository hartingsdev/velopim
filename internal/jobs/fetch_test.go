package jobs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"velopim/internal/db"
	"velopim/internal/dbtest"
)

// The job arguments must carry their insert options, or uniqueness and the
// retry limit silently do not apply.
var (
	_ river.JobArgsWithInsertOpts = FetchSourceArgs{}
	_ river.JobArgsWithInsertOpts = ScheduleArgs{}
	_ river.JobArgsWithInsertOpts = SyncArgs{}
)

func one(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var out *string
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	if out == nil {
		return "<null>"
	}
	return *out
}

func TestFetchSource(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	file := "Nr;Marke;Name;EK\n1;Beispielrad;City 3;500,00\n"
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "broken", status)
			return
		}
		_, _ = io.WriteString(w, file)
	}))
	defer srv.Close()

	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind, connector_config, fetch_config) VALUES
		  ('hartje', 'Hartje', 'csvfile', '{"delimiter": ";", "decimal_comma": true, "columns": {"sku": "Nr", "brand": "Marke", "name": "Name", "cost": "EK"}}',
		   jsonb_build_object('url', $1::text, 'interval_minutes', 60, 'auth', 'bearer', 'secret_env', 'VELOPIM_FETCH_TOKEN')),
		  ('manual', 'Nur Upload', 'csvfile', '{"delimiter": ";", "columns": {"sku": "Nr"}}', '{}')`, srv.URL+"/feed.csv"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 1, 10, 6, 0, 0, 0, time.UTC)
	f := &Fetcher{Pool: pool, RawDir: t.TempDir(), HTTP: srv.Client(), Now: func() time.Time { return now },
		Getenv: func(k string) string { return map[string]string{"VELOPIM_FETCH_TOKEN": "tok-123"}[k] }}

	// A source that was never fetched is due; one without address never is.
	due, err := DueSources(ctx, db.New(pool), now)
	if err != nil || strings.Join(due, ",") != "hartje" {
		t.Fatalf("due sources: %v %v", due, err)
	}
	if _, err := f.FetchSource(ctx, "manual"); !errors.Is(err, ErrNothingToFetch) {
		t.Errorf("source without address: got %v, want ErrNothingToFetch", err)
	}

	// Fetch and import.
	res, err := f.FetchSource(ctx, "hartje")
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 1 || res.New != 1 || res.PendingApproval != 1 {
		t.Errorf("import result: %+v", res)
	}
	if got := one(t, pool, `SELECT (last_fetch_at = $1)::text || ' ' || coalesce(last_fetch_error, 'ok') FROM source WHERE id = 'hartje'`, now); got != "true ok" {
		t.Errorf("stored outcome: %s", got)
	}
	if got := one(t, pool, `SELECT name FROM supplier_item WHERE source_id = 'hartje'`); got != "City 3" {
		t.Errorf("imported article: %s", got)
	}

	// Not due again until the interval has passed.
	for _, c := range []struct {
		after time.Duration
		want  string
	}{{59 * time.Minute, ""}, {60 * time.Minute, "hartje"}, {3 * time.Hour, "hartje"}} {
		due, _ := DueSources(ctx, db.New(pool), now.Add(c.after))
		if strings.Join(due, ",") != c.want {
			t.Errorf("due after %s: %v, want %q", c.after, due, c.want)
		}
	}

	// The server fails twice: the error is stored and there is one task.
	status = http.StatusServiceUnavailable
	for range 2 {
		if _, err := f.FetchSource(ctx, "hartje"); err == nil {
			t.Fatal("failing fetch returned no error")
		}
	}
	if got := one(t, pool, `SELECT last_fetch_error FROM source WHERE id = 'hartje'`); !strings.Contains(got, "status 503") {
		t.Errorf("stored error: %s", got)
	}
	if got := one(t, pool, `SELECT count(*) || ' ' || max(title) || ' ' || max(priority) FROM task WHERE kind = 'fetch_failed' AND status = 'open'`); got != "1 Abholen fehlgeschlagen: Hartje 1" {
		t.Errorf("failure task: %s", got)
	}
	if n := one(t, pool, `SELECT count(*)::text FROM import_run`); n != "1" {
		t.Errorf("failed fetches created import runs: %s", n)
	}

	// A file the connector cannot read is a failure too.
	status, file = http.StatusOK, "<html>Wartung</html>"
	if _, err := f.FetchSource(ctx, "hartje"); err == nil {
		t.Error("unreadable file returned no error")
	}

	// It works again: error and task are gone, the changed price arrives.
	file = "Nr;Marke;Name;EK\n1;Beispielrad;City 3;480,00\n"
	if _, err := f.FetchSource(ctx, "hartje"); err != nil {
		t.Fatal(err)
	}
	if got := one(t, pool, `SELECT coalesce(s.last_fetch_error, 'ok') || ' ' || (SELECT count(*) FROM task WHERE kind = 'fetch_failed' AND status = 'open') || ' ' ||
		(SELECT cost_cents FROM supplier_item WHERE source_id = 'hartje') FROM source s WHERE s.id = 'hartje'`); got != "ok 0 48000" {
		t.Errorf("after recovery: %s", got)
	}

	if _, err := f.FetchSource(ctx, "nobody"); err == nil {
		t.Error("unknown source returned no error")
	}
}
