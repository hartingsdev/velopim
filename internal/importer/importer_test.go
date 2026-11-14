package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/connector/csvfile"
	"velopim/internal/tasks"
)

// These tests need PostgreSQL. Set TEST_DATABASE_URL (e.g. to DATABASE_URL
// from .env); without it they are skipped. Each test gets its own schema.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	pool, err := pgxpool.New(ctx, url+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	migration, err := os.ReadFile("../../db/migrations/00001_core_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(migration), "-- +goose Down")
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind, availability_mapping) VALUES ('example', 'Beispiel', 'csvfile', '[
		  {"name": "in stock", "when": [{"field": "Bestand", "op": "gt", "value": "0"}], "status": "in_stock", "quantity_field": "Bestand"},
		  {"name": "with date", "when": [{"field": "Liefertermin", "op": "not_empty"}], "status": "preorder", "date_field": "Liefertermin", "date_layout": "02.01.2006"},
		  {"name": "sold out", "when": [{"field": "Bestand", "op": "lte", "value": "0"}], "status": "unavailable"}
		]');
		INSERT INTO rule (name, action_type, outcome, conditions, guard) VALUES
		  ('new products need approval', 'new_product', 'approval', '[]', false),
		  ('availability is automatic', 'availability_status_change', 'auto', '[]', false),
		  ('small price change with healthy margin', 'price_change', 'auto',
		   '[{"fact": "price_change_abs_pct", "op": "lt", "value": 5}, {"fact": "margin_pct", "op": "gt", "value": 20}]', false);
		INSERT INTO pricing_rule (config) VALUES ('{"mode": "markup", "value": 1.5, "vat_pct": 19}');
	`); err != nil {
		t.Fatal(err)
	}
	return pool
}

const header = "Artikelnr;EAN;Marke;Bezeichnung;Modelljahr;Farbe;Rahmenhoehe;EK;UVP;Bestand;Liefertermin\n"

func file(rows ...string) *strings.Reader {
	return strings.NewReader(header + strings.Join(rows, "\n") + "\n")
}

const (
	row1 = "100-001;4006381333931;Beispielrad;Trekking 5.4;2027;schwarz;52;1.000,00;1.999,00;4;"
	row2 = "100-002;4006381333948;Beispielrad;Trekking 5.4;2027;schwarz;56;1.000,00;1.999,00;0;15.03.2027"
	row3 = "100-003;;Beispielrad;Trekking 5.4;2027;silber;48;1.000,00;1.999,00;2;"
)

func newConnector(t *testing.T) *csvfile.Connector {
	t.Helper()
	c, err := csvfile.New(csvfile.Config{SourceID: "example", Delimiter: ";", DecimalComma: true,
		Columns: map[string]string{csvfile.SKU: "Artikelnr", csvfile.EAN: "EAN", csvfile.Brand: "Marke",
			csvfile.Name: "Bezeichnung", csvfile.ModelYear: "Modelljahr", csvfile.Color: "Farbe",
			csvfile.FrameSize: "Rahmenhoehe", csvfile.Cost: "EK", csvfile.RRP: "UVP"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestImportPipeline(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	conn := newConnector(t)
	opt := Options{RawDir: t.TempDir(), MaxDropPct: 50}

	// First delivery: everything is new and needs approval.
	res, err := Run(ctx, pool, conn, file(row1, row2, row3), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "done" || res.Records != 3 || res.New != 3 || res.PendingApproval != 3 || res.AutoApplied != 0 || res.Issues != 1 {
		t.Fatalf("first run: %+v", res)
	}
	if n := count(t, pool, "raw_record"); n != 3 {
		t.Errorf("raw records = %d, want 3", n)
	}
	if n := count(t, pool, "task WHERE kind = 'approve_action' AND status = 'open'"); n != 3 {
		t.Errorf("approval tasks = %d, want 3", n)
	}
	if n := count(t, pool, "task WHERE kind = 'data_issue'"); n != 1 {
		t.Errorf("data issue tasks = %d, want 1 (missing EAN)", n)
	}
	var path string
	if err := pool.QueryRow(ctx, `SELECT file_path FROM import_run WHERE id = $1`, res.RunID).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(b), header) {
		t.Errorf("original file not stored unchanged: %v", err)
	}

	// Same delivery again: nothing happens, no new raw versions.
	res, err = Run(ctx, pool, conn, file(row1, row2, row3), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 0 || res.Changed != 0 || res.PendingApproval != 0 || res.AutoApplied != 0 || res.QuantityChanges != 0 {
		t.Fatalf("repeated run must be a no-op: %+v", res)
	}
	if n := count(t, pool, "raw_record"); n != 3 {
		t.Errorf("raw records = %d after identical import, want 3", n)
	}

	// Third delivery: small price change and sold out on article 1 (both
	// automatic), quantity change on article 3, article 2 is gone.
	changed1 := "100-001;4006381333931;Beispielrad;Trekking 5.4;2027;schwarz;52;1.010,00;1.999,00;0;"
	changed3 := "100-003;;Beispielrad;Trekking 5.4;2027;silber;48;1.000,00;1.999,00;1;"
	res, err = Run(ctx, pool, conn, file(changed1, changed3), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "done" || res.Changed != 1 || res.AutoApplied != 2 || res.Removed != 1 || res.PendingApproval != 1 || res.QuantityChanges != 2 {
		t.Fatalf("third run: %+v", res)
	}
	if n := count(t, pool, "raw_record"); n != 5 {
		t.Errorf("raw records = %d, want 5 (two articles changed)", n)
	}
	var rule, reason, oldV, newV string
	if err := pool.QueryRow(ctx,
		`SELECT r.name, a.decision_reason, a.old_value, a.new_value FROM action a JOIN rule r ON r.id = a.rule_id
		 WHERE a.type = 'price_change' AND a.status = 'auto_applied'`).Scan(&rule, &reason, &oldV, &newV); err != nil {
		t.Fatal(err)
	}
	if rule != "small price change with healthy margin" || oldV != "100000" || newV != "101000" || reason == "" {
		t.Errorf("auto price action: rule=%q %s -> %s (%s)", rule, oldV, newV, reason)
	}
	if n := count(t, pool, "field_change WHERE field = 'cost_cents' AND old_value = '100000' AND new_value = '101000' AND origin = 'import' AND import_run_id = $1", res.RunID); n != 1 {
		t.Errorf("field change for the purchase price missing")
	}
	if n := count(t, pool, "supplier_item WHERE sku = '100-002' AND NOT active"); n != 1 {
		t.Errorf("removed article is still active")
	}
	if n := count(t, pool, "task t JOIN action a ON a.id = t.action_id WHERE a.type = 'deactivation' AND t.status = 'open'"); n != 1 {
		t.Errorf("no approval task for the removed article")
	}

	// Large price change: no rule matches, so it needs approval. A second
	// change replaces the first pending one.
	for _, ek := range []string{"1.300,00", "1.400,00"} {
		big := "100-001;4006381333931;Beispielrad;Trekking 5.4;2027;schwarz;52;" + ek + ";1.999,00;0;"
		if _, err := Run(ctx, pool, conn, file(big, changed3), opt); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, "action WHERE type = 'price_change' AND status = 'pending_approval'"); n != 1 {
		t.Errorf("pending price actions = %d, want 1", n)
	}
	if n := count(t, pool, "action WHERE type = 'price_change' AND status = 'superseded'"); n != 1 {
		t.Errorf("superseded price actions = %d, want 1", n)
	}
	if n := count(t, pool, "task t JOIN action a ON a.id = t.action_id WHERE a.type = 'price_change' AND t.status = 'open'"); n != 1 {
		t.Errorf("open price tasks = %d, want 1", n)
	}

	// Approving the task decides the action; doing it twice fails.
	var taskID int64
	if err := pool.QueryRow(ctx,
		`SELECT t.id FROM task t JOIN action a ON a.id = t.action_id WHERE a.type = 'price_change' AND t.status = 'open'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Decide(ctx, pool, taskID, true, "robert", "passt"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "action WHERE type = 'price_change' AND status = 'approved' AND decided_by = 'robert' AND decision_note = 'passt'"); n != 1 {
		t.Errorf("action was not approved with actor and note")
	}
	if err := tasks.Decide(ctx, pool, taskID, true, "robert", ""); !errors.Is(err, tasks.ErrNotOpen) {
		t.Errorf("second decision: got %v, want ErrNotOpen", err)
	}
}

func TestImportHeldOnSuspiciousDrop(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	conn := newConnector(t)
	opt := Options{RawDir: t.TempDir()} // default: hold above 20 percent

	if _, err := Run(ctx, pool, conn, file(row1, row2, row3), opt); err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, pool, conn, file(row1), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "held" || res.Removed != 0 {
		t.Fatalf("truncated delivery must be held: %+v", res)
	}
	if n := count(t, pool, "supplier_item WHERE active"); n != 3 {
		t.Errorf("active articles = %d, want 3 (nothing deactivated)", n)
	}
	if n := count(t, pool, "task WHERE kind = 'import_held' AND priority = 1"); n != 1 {
		t.Errorf("no task for the held import")
	}
}

func TestImportUnknownSourceFails(t *testing.T) {
	pool := testDB(t)
	c, _ := csvfile.New(csvfile.Config{SourceID: "nope", Delimiter: ";", Columns: map[string]string{csvfile.SKU: "Artikelnr"}})
	if _, err := Run(context.Background(), pool, c, file(row1), Options{RawDir: t.TempDir()}); err == nil {
		t.Fatal("expected an error for an unconfigured source")
	}
	if n := count(t, pool, "import_run"); n != 0 {
		t.Errorf("failed run left %d import runs behind", n)
	}
}
