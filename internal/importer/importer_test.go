package importer

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/connector/csvfile"
	"velopim/internal/dbtest"
	"velopim/internal/tasks"
)

// These tests need PostgreSQL, see dbtest.New.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()
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

	// First delivery: three new articles of one model, so one approval.
	res, err := Run(ctx, pool, conn, file(row1, row2, row3), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "done" || res.Records != 3 || res.New != 3 || res.PendingApproval != 1 || res.AutoApplied != 0 || res.Issues != 1 {
		t.Fatalf("first run: %+v", res)
	}
	if n := count(t, pool, "raw_record"); n != 3 {
		t.Errorf("raw records = %d, want 3", n)
	}
	if n := count(t, pool, "task WHERE kind = 'approve_action' AND status = 'open'"); n != 1 {
		t.Errorf("approval tasks = %d, want 1 for the model", n)
	}
	if n := count(t, pool, "action_item"); n != 3 {
		t.Errorf("articles on the model action = %d, want 3", n)
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

func TestNewModelsGetOneTaskEach(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	conn := newConnector(t)
	opt := Options{RawDir: t.TempDir()}
	city := "200-001;;Beispielrad;City 3;2027;rot;50;500,00;899,00;1;"

	openTask := func(model string) (id int64, title string, items int) {
		t.Helper()
		err := pool.QueryRow(ctx, `
			SELECT t.id, t.title, (SELECT count(*) FROM action_item ai WHERE ai.action_id = a.id)
			FROM task t JOIN action a ON a.id = t.action_id
			WHERE a.type = 'new_product' AND t.status = 'open' AND a.new_value = $1`, model).Scan(&id, &title, &items)
		if err != nil {
			t.Fatalf("open task for %s: %v", model, err)
		}
		return id, title, items
	}

	// Two articles of one model and one of another: two tasks.
	res, err := Run(ctx, pool, conn, file(row1, row2, city), opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 3 || res.PendingApproval != 2 {
		t.Fatalf("first run: %+v", res)
	}
	if _, title, items := openTask("Beispielrad Trekking 5.4 (2027)"); items != 2 || title != "Neues Modell prüfen: Beispielrad Trekking 5.4 (2027), 2 Varianten" {
		t.Errorf("trekking task: %q with %d articles", title, items)
	}
	if _, title, items := openTask("Beispielrad City 3 (2027)"); items != 1 || title != "Neues Modell prüfen: Beispielrad City 3 (2027), 1 Variante" {
		t.Errorf("city task: %q with %d articles", title, items)
	}

	// A further article of the undecided model joins its task instead of
	// opening a second one.
	if _, err := Run(ctx, pool, conn, file(row1, row2, row3, city), opt); err != nil {
		t.Fatal(err)
	}
	taskID, title, items := openTask("Beispielrad Trekking 5.4 (2027)")
	if items != 3 || title != "Neues Modell prüfen: Beispielrad Trekking 5.4 (2027), 3 Varianten" {
		t.Errorf("after a further article: %q with %d articles", title, items)
	}
	if n := count(t, pool, "task t JOIN action a ON a.id = t.action_id WHERE a.type = 'new_product' AND t.status = 'open'"); n != 2 {
		t.Errorf("open new-product tasks = %d, want 2", n)
	}
	if n := count(t, pool, "action WHERE type = 'new_product' AND status = 'superseded'"); n != 1 {
		t.Errorf("superseded model actions = %d, want 1", n)
	}

	// Once the model is approved, a later article is a new variant.
	if err := tasks.Decide(ctx, pool, taskID, true, "robert", ""); err != nil {
		t.Fatal(err)
	}
	row4 := "100-004;;Beispielrad;Trekking 5.4;2027;silber;52;1.000,00;1.999,00;2;"
	if _, err := Run(ctx, pool, conn, file(row1, row2, row3, row4, city), opt); err != nil {
		t.Fatal(err)
	}
	if _, title, items := openTask("Beispielrad Trekking 5.4 (2027)"); items != 1 || title != "Neue Varianten prüfen: Beispielrad Trekking 5.4 (2027), 1 Variante" {
		t.Errorf("after approval: %q with %d articles", title, items)
	}

	// The same model in another model year is a model of its own.
	next := "100-101;;Beispielrad;Trekking 5.4;2028;schwarz;52;1.000,00;1.999,00;2;"
	if _, err := Run(ctx, pool, conn, file(row1, row2, row3, row4, city, next), opt); err != nil {
		t.Fatal(err)
	}
	if _, title, _ := openTask("Beispielrad Trekking 5.4 (2028)"); title != "Neues Modell prüfen: Beispielrad Trekking 5.4 (2028), 1 Variante" {
		t.Errorf("next model year: %q", title)
	}
}

func TestNewModelAutoRule(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO rule (name, action_type, scope_brand, outcome) VALUES ('brand is trusted', 'new_product', 'Beispielrad', 'auto')`); err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, pool, newConnector(t), file(row1, row2), Options{RawDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoApplied != 1 || res.PendingApproval != 0 {
		t.Fatalf("run: %+v", res)
	}
	if n := count(t, pool, "action a JOIN rule r ON r.id = a.rule_id WHERE a.status = 'auto_applied' AND r.name = 'brand is trusted'"); n != 1 {
		t.Errorf("auto action with its rule missing")
	}
	if n := count(t, pool, "task WHERE kind = 'approve_action'"); n != 0 {
		t.Errorf("auto-applied model must not create a task")
	}
}
