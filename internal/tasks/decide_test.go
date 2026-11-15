package tasks_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/catalog"
	"velopim/internal/connector/csvfile"
	"velopim/internal/dbtest"
	"velopim/internal/importer"
	"velopim/internal/tasks"
)

const header = "Artikelnr;EAN;Marke;Bezeichnung;Modelljahr;Farbe;Rahmenhoehe;EK;UVP;Bestand\n"

func runImport(t *testing.T, pool *pgxpool.Pool, rows ...string) {
	t.Helper()
	conn, err := csvfile.New(csvfile.Config{SourceID: "example", Delimiter: ";", DecimalComma: true,
		Columns: map[string]string{csvfile.SKU: "Artikelnr", csvfile.EAN: "EAN", csvfile.Brand: "Marke",
			csvfile.Name: "Bezeichnung", csvfile.ModelYear: "Modelljahr", csvfile.Color: "Farbe",
			csvfile.FrameSize: "Rahmenhoehe", csvfile.Cost: "EK", csvfile.RRP: "UVP"}})
	if err != nil {
		t.Fatal(err)
	}
	file := strings.NewReader(header + strings.Join(rows, "\n") + "\n")
	if _, err := importer.Run(context.Background(), pool, conn, file, importer.Options{RawDir: t.TempDir(), MaxDropPct: 100}); err != nil {
		t.Fatal(err)
	}
}

func openModelTask(t *testing.T, pool *pgxpool.Pool, model string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `SELECT t.id FROM task t JOIN action a ON a.id = t.action_id
		WHERE a.type = 'new_product' AND t.status = 'open' AND a.new_value = $1`, model).Scan(&id)
	if err != nil {
		t.Fatalf("open task for %s: %v", model, err)
	}
	return id
}

func query(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
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

func TestApprovingAModelCreatesProducts(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO pricing_rule (config) VALUES ('{"mode": "markup", "value": 1.5, "vat_pct": 19}');
	`); err != nil {
		t.Fatal(err)
	}
	const (
		black52  = "100-001;4006381333931;Beispielrad;Trekking 5.4;2027;schwarz;52;1.000,00;1.999,00;4"
		black56  = "100-002;4006381333948;Beispielrad;Trekking 5.4;2027;schwarz;56;1.000,00;1.999,00;0"
		silver48 = "100-003;;Beispielrad;Trekking 5.4;2027;silber;48;1.000,00;1.999,00;2"
		silver52 = "100-004;;Beispielrad;Trekking 5.4;2027;silber;52;1.000,00;1.999,00;2"
		model    = "Beispielrad Trekking 5.4 (2027)"
	)

	runImport(t, pool, black52, black56, silver48)
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, model), true, "robert", "", []catalog.Axis{catalog.AxisColor}); err != nil {
		t.Fatal(err)
	}

	// Split by color: two draft products, frame size is the only option.
	got := query(t, pool, `SELECT string_agg(name || ' ' || status || ' ' || option_names::text || ' ' ||
		(SELECT count(*) FROM variant v WHERE v.product_id = p.id), ' | ' ORDER BY name) FROM product p`)
	if want := `Trekking 5.4 schwarz draft ["Rahmenhöhe"] 2 | Trekking 5.4 silber draft ["Rahmenhöhe"] 1`; got != want {
		t.Errorf("products:\n got  %s\n want %s", got, want)
	}
	// Own article numbers, price from the rule (1000 € × 1.5 × 1.19), RRP as
	// compare-at price, supplier article linked as preferred.
	got = query(t, pool, `SELECT string_agg(v.sku || ' ' || v.option_values::text || ' ' || v.price_cents || ' ' ||
		v.compare_at_cents || ' ' || s.sku || ' ' || vs.preferred, ' | ' ORDER BY v.sku)
		FROM variant v JOIN variant_supplier_item vs ON vs.variant_id = v.id JOIN supplier_item s ON s.id = vs.supplier_item_id`)
	want := `VP-100001 ["52"] 178500 199900 100-001 true | VP-100002 ["56"] 178500 199900 100-002 true | VP-100003 ["48"] 178500 199900 100-003 true`
	if got != want {
		t.Errorf("variants:\n got  %s\n want %s", got, want)
	}
	// Provenance: who created what through which action.
	got = query(t, pool, `SELECT string_agg(DISTINCT entity_type || '.' || field || ' ' || origin || ' ' || actor, ' | ')
		FROM field_change WHERE action_id IS NOT NULL`)
	if want := "product.status user robert | variant.price_cents user robert | variant.sku user robert"; got != want {
		t.Errorf("field changes:\n got  %s\n want %s", got, want)
	}

	// A later article joins the existing product of its color; the cut of
	// the model stays even though no split axes are passed. Without a
	// pricing rule the variant has no price.
	if _, err := pool.Exec(ctx, `DELETE FROM pricing_rule`); err != nil {
		t.Fatal(err)
	}
	runImport(t, pool, black52, black56, silver48, silver52)
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, model), true, "robert", "", nil); err != nil {
		t.Fatal(err)
	}
	got = query(t, pool, `SELECT string_agg(p.name || ' ' || v.sku || ' ' || coalesce(v.price_cents::text, 'no price'), ' | ' ORDER BY v.sku)
		FROM variant v JOIN product p ON p.id = v.product_id WHERE p.name = 'Trekking 5.4 silber'`)
	if want := "Trekking 5.4 silber VP-100003 178500 | Trekking 5.4 silber VP-100004 no price"; got != want {
		t.Errorf("after a further article:\n got  %s\n want %s", got, want)
	}
	if n := query(t, pool, `SELECT count(*)::text FROM product`); n != "2" {
		t.Errorf("products = %s, want 2", n)
	}
}

func TestBlockedModelStaysOpen(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile')`); err != nil {
		t.Fatal(err)
	}
	// Two articles with the same color and frame size cannot be told apart.
	runImport(t, pool,
		"200-001;;Beispielrad;City 3;2027;rot;50;500,00;899,00;1",
		"200-002;;Beispielrad;City 3;2027;rot;50;520,00;899,00;1")
	taskID := openModelTask(t, pool, "Beispielrad City 3 (2027)")

	err := tasks.Decide(ctx, pool, taskID, true, "robert", "", nil)
	var blocked *catalog.BlockedError
	if !errors.As(err, &blocked) || len(blocked.Issues) != 1 || !strings.Contains(blocked.Issues[0], "nicht unterscheidbar") {
		t.Fatalf("got %v, want a BlockedError about indistinguishable articles", err)
	}
	if err := tasks.Decide(ctx, pool, taskID, true, "robert", "", []catalog.Axis{"bogus"}); !errors.As(err, &blocked) {
		t.Errorf("unknown axis: got %v, want a BlockedError", err)
	}
	got := query(t, pool, `SELECT t.status || ' ' || a.status || ' ' || (SELECT count(*) FROM product)
		FROM task t JOIN action a ON a.id = t.action_id WHERE t.id = $1`, taskID)
	if got != "open pending_approval 0" {
		t.Errorf("after a blocked approval: %s, want everything unchanged", got)
	}

	// Rejecting works and creates nothing.
	if err := tasks.Decide(ctx, pool, taskID, false, "robert", "doppelt", nil); err != nil {
		t.Fatal(err)
	}
	if n := query(t, pool, `SELECT count(*)::text FROM product`); n != "0" {
		t.Errorf("rejected model created %s products", n)
	}
}

func TestCostChangeMovesTheSalesPriceByTheSameAmount(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO pricing_rule (config) VALUES ('{"mode": "markup", "value": 1.5, "vat_pct": 19}');
	`); err != nil {
		t.Fatal(err)
	}
	row := func(sku, size, cost string) string {
		return sku + ";;Beispielrad;Trekking 5.4;2027;schwarz;" + size + ";" + cost + ";1.999,00;4"
	}
	// Two variants, both created at 1000 € × 1.5 × 1.19 = 1785 €.
	runImport(t, pool, row("100-001", "52", "1.000,00"), row("100-002", "56", "1.000,00"))
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, "Beispielrad Trekking 5.4 (2027)"), true, "robert", "", nil); err != nil {
		t.Fatal(err)
	}
	price := func(supplierSKU string) string {
		return query(t, pool, `SELECT v.price_cents::text FROM variant v JOIN variant_supplier_item vs ON vs.variant_id = v.id
			JOIN supplier_item s ON s.id = vs.supplier_item_id WHERE s.sku = $1`, supplierSKU)
	}
	priceTask := func(supplierSKU string) (id int64, proposal string) {
		t.Helper()
		err := pool.QueryRow(ctx, `SELECT t.id, a.old_value || '>' || a.new_value || ' ' || a.base_price_cents || '>' || a.proposed_price_cents
			FROM task t JOIN action a ON a.id = t.action_id JOIN supplier_item s ON s.id = a.supplier_item_id
			WHERE a.field = 'cost_cents' AND t.status = 'open' AND s.sku = $1`, supplierSKU).Scan(&id, &proposal)
		if err != nil {
			t.Fatalf("open price task for %s: %v", supplierSKU, err)
		}
		return id, proposal
	}

	// The supplier lowers the purchase price by 5 €: the proposal lowers the
	// sales price by 5 €, not by the markup on 5 €. Nothing changes yet.
	runImport(t, pool, row("100-001", "52", "995,00"), row("100-002", "56", "1.000,00"))
	if _, proposal := priceTask("100-001"); proposal != "100000>99500 178500>178000" {
		t.Errorf("proposal = %s", proposal)
	}
	if p := price("100-001"); p != "178500" {
		t.Errorf("price changed before approval: %s", p)
	}

	// A second change before the decision replaces the task and covers both
	// steps: 1000 € → 980 €, so 20 € off.
	runImport(t, pool, row("100-001", "52", "980,00"), row("100-002", "56", "1.000,00"))
	taskID, proposal := priceTask("100-001")
	if proposal != "100000>98000 178500>176500" {
		t.Errorf("carried-over proposal = %s", proposal)
	}
	if err := tasks.Decide(ctx, pool, taskID, true, "robert", "", nil); err != nil {
		t.Fatal(err)
	}
	if p := price("100-001"); p != "176500" {
		t.Errorf("price after approval = %s, want 176500", p)
	}
	if p := price("100-002"); p != "178500" {
		t.Errorf("other variant changed: %s", p)
	}
	got := query(t, pool, `SELECT old_value || '>' || new_value || ' ' || origin || ' ' || actor FROM field_change
		WHERE entity_type = 'variant' AND field = 'price_cents' AND old_value IS NOT NULL`)
	if got != "178500>176500 user robert" {
		t.Errorf("field change = %s", got)
	}

	// An increase is proposed the same way; rejecting it leaves the price.
	runImport(t, pool, row("100-001", "52", "1.030,00"), row("100-002", "56", "1.000,00"))
	taskID, proposal = priceTask("100-001")
	if proposal != "98000>103000 176500>181500" {
		t.Errorf("increase proposal = %s", proposal)
	}
	if err := tasks.Decide(ctx, pool, taskID, false, "robert", "Lagerware", nil); err != nil {
		t.Fatal(err)
	}
	if p := price("100-001"); p != "176500" {
		t.Errorf("price after rejection = %s", p)
	}

	// With a rule, decreases are passed on automatically and increases
	// still need approval.
	if _, err := pool.Exec(ctx, `INSERT INTO rule (name, action_type, scope_source_id, conditions, outcome) VALUES
		('pass on decreases', 'price_change', 'example', '[{"fact": "cost_change_cents", "op": "lt", "value": 0}]', 'auto')`); err != nil {
		t.Fatal(err)
	}
	runImport(t, pool, row("100-001", "52", "1.030,00"), row("100-002", "56", "990,00"))
	if p := price("100-002"); p != "177500" {
		t.Errorf("automatic decrease: price = %s, want 177500", p)
	}
	got = query(t, pool, `SELECT fc.origin || ' ' || a.status || ' ' || r.name FROM field_change fc
		JOIN action a ON a.id = fc.action_id JOIN rule r ON r.id = a.rule_id WHERE fc.origin = 'rule'`)
	if got != "rule auto_applied pass on decreases" {
		t.Errorf("automatic change: %s", got)
	}
	runImport(t, pool, row("100-001", "52", "1.030,00"), row("100-002", "56", "1.010,00"))
	if _, proposal := priceTask("100-002"); proposal != "99000>101000 177500>179500" {
		t.Errorf("increase with the rule in place: %s", proposal)
	}
	if p := price("100-002"); p != "177500" {
		t.Errorf("increase was applied without approval: %s", p)
	}
}
