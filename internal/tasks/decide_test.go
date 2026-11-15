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
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, model), tasks.Decision{Approve: true, Actor: "robert", SplitBy: []catalog.Axis{catalog.AxisColor}}); err != nil {
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
	want := `10001 ["52"] 178500 199900 100-001 true | 10002 ["56"] 178500 199900 100-002 true | 10003 ["48"] 178500 199900 100-003 true`
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
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, model), tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
		t.Fatal(err)
	}
	got = query(t, pool, `SELECT string_agg(p.name || ' ' || v.sku || ' ' || coalesce(v.price_cents::text, 'no price'), ' | ' ORDER BY v.sku)
		FROM variant v JOIN product p ON p.id = v.product_id WHERE p.name = 'Trekking 5.4 silber'`)
	if want := "Trekking 5.4 silber 10003 178500 | Trekking 5.4 silber 10004 no price"; got != want {
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

	err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert"})
	var blocked *catalog.BlockedError
	if !errors.As(err, &blocked) || len(blocked.Issues) != 1 || !strings.Contains(blocked.Issues[0], "nicht unterscheidbar") {
		t.Fatalf("got %v, want a BlockedError about indistinguishable articles", err)
	}
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert", SplitBy: []catalog.Axis{"bogus"}}); !errors.As(err, &blocked) {
		t.Errorf("unknown axis: got %v, want a BlockedError", err)
	}
	got := query(t, pool, `SELECT t.status || ' ' || a.status || ' ' || (SELECT count(*) FROM product)
		FROM task t JOIN action a ON a.id = t.action_id WHERE t.id = $1`, taskID)
	if got != "open pending_approval 0" {
		t.Errorf("after a blocked approval: %s, want everything unchanged", got)
	}

	// Rejecting works and creates nothing.
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: false, Actor: "robert", Note: "doppelt"}); err != nil {
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
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, "Beispielrad Trekking 5.4 (2027)"), tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
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
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
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
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: false, Actor: "robert", Note: "Lagerware"}); err != nil {
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

func TestCorrectingPricesAtApproval(t *testing.T) {
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
	prices := func() string {
		return query(t, pool, `SELECT string_agg(s.sku || '=' || v.price_cents || '/' || coalesce(v.compare_at_cents::text, '-'), ' ' ORDER BY s.sku)
			FROM variant v JOIN variant_supplier_item vs ON vs.variant_id = v.id JOIN supplier_item s ON s.id = vs.supplier_item_id`)
	}
	runImport(t, pool, row("100-001", "52", "1.000,00"), row("100-002", "56", "1.000,00"))
	taskID := openModelTask(t, pool, "Beispielrad Trekking 5.4 (2027)")

	// A price of zero is refused and leaves the task open.
	var blocked *catalog.BlockedError
	err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert", VariantPrices: map[string]int64{"100-001": 0}})
	if !errors.As(err, &blocked) {
		t.Fatalf("zero price: got %v, want a BlockedError", err)
	}

	// One variant gets a corrected price, the other keeps the calculated
	// one. Above the RRP there is no compare-at price.
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert",
		VariantPrices: map[string]int64{"100-001": 209900}}); err != nil {
		t.Fatal(err)
	}
	if got := prices(); got != "100-001=209900/- 100-002=178500/199900" {
		t.Errorf("prices after approval: %s", got)
	}
	got := query(t, pool, `SELECT string_agg(new_value || ' ' || reason, ' | ' ORDER BY new_value) FROM field_change WHERE field = 'price_cents'`)
	if got != "178500 Modell freigegeben | 209900 Preis bei der Freigabe festgelegt" {
		t.Errorf("field changes: %s", got)
	}

	// Purchase price drops by 20 €; the proposal would be 1765 €, the
	// person decides on 1749 € instead.
	runImport(t, pool, row("100-001", "52", "1.000,00"), row("100-002", "56", "980,00"))
	err = pool.QueryRow(ctx, `SELECT t.id FROM task t JOIN action a ON a.id = t.action_id WHERE a.field = 'cost_cents' AND t.status = 'open'`).Scan(&taskID)
	if err != nil {
		t.Fatal(err)
	}
	corrected := int64(174900)
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert", PriceCents: &corrected}); err != nil {
		t.Fatal(err)
	}
	if got := prices(); got != "100-001=209900/- 100-002=174900/199900" {
		t.Errorf("prices after corrected approval: %s", got)
	}
	got = query(t, pool, `SELECT fc.old_value || '>' || fc.new_value || ' ' || fc.reason || ' ' || a.base_price_cents || '>' || a.proposed_price_cents
		FROM field_change fc JOIN action a ON a.id = fc.action_id WHERE a.field = 'cost_cents'`)
	if got != "178500>174900 Preis bei der Freigabe korrigiert 178500>174900" {
		t.Errorf("corrected price change: %s", got)
	}
}

func TestApprovedSupplierChangesReachTheCatalog(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO pricing_rule (config) VALUES ('{"mode": "markup", "value": 1.5, "vat_pct": 19}');
		INSERT INTO rule (name, action_type, outcome) VALUES ('images are automatic', 'images_change', 'auto');
	`); err != nil {
		t.Fatal(err)
	}
	conn, err := csvfile.New(csvfile.Config{SourceID: "example", Delimiter: ";", DecimalComma: true,
		Columns: map[string]string{csvfile.SKU: "Nr", csvfile.EAN: "EAN", csvfile.Brand: "Marke", csvfile.Name: "Name",
			csvfile.ModelYear: "Jahr", csvfile.Color: "Farbe", csvfile.FrameSize: "RH", csvfile.Cost: "EK", csvfile.RRP: "UVP",
			csvfile.Description: "Text", csvfile.Images: "Bilder"}})
	if err != nil {
		t.Fatal(err)
	}
	load := func(rows ...string) {
		t.Helper()
		file := strings.NewReader("Nr;EAN;Marke;Name;Jahr;Farbe;RH;EK;UVP;Text;Bilder\n" + strings.Join(rows, "\n") + "\n")
		if _, err := importer.Run(ctx, pool, conn, file, importer.Options{RawDir: t.TempDir(), MaxDropPct: 100}); err != nil {
			t.Fatal(err)
		}
	}
	approve := func(field string) {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `SELECT t.id FROM task t JOIN action a ON a.id = t.action_id
			WHERE a.field = $1 AND t.status = 'open'`, field).Scan(&id); err != nil {
			t.Fatalf("open task for %s: %v", field, err)
		}
		if err := tasks.Decide(ctx, pool, id, tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
			t.Fatal(err)
		}
	}
	state := func() string {
		return query(t, pool, `SELECT p.description_html || ' | ' || p.image_urls::text || ' | ' ||
			string_agg(v.ean || ' ' || v.price_cents || '/' || coalesce(v.compare_at_cents::text, '-'), ', ' ORDER BY v.sku)
			FROM product p JOIN variant v ON v.product_id = p.id GROUP BY p.id`)
	}

	load("1;;Beispielrad;City 3;2027;rot;50;1.000,00;1.999,00;Alter Text;https://example.test/a.jpg",
		"2;;Beispielrad;City 3;2027;rot;54;1.000,00;1.999,00;Alter Text;https://example.test/a.jpg")
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, "Beispielrad City 3 (2027)"), tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
		t.Fatal(err)
	}
	if got, want := state(), `Alter Text | ["https://example.test/a.jpg"] |  178500/199900,  178500/199900`; got != want {
		t.Fatalf("initial state:\n got  %s\n want %s", got, want)
	}

	// Article 1 gets a new text, a second image, an EAN and a lower RRP.
	// Images follow automatically by rule; the rest waits for approval.
	load("1;4006381333931;Beispielrad;City 3;2027;rot;50;1.000,00;1.700,00;Neuer Text;https://example.test/a.jpg|https://example.test/b.jpg",
		"2;;Beispielrad;City 3;2027;rot;54;1.000,00;1.999,00;Alter Text;https://example.test/a.jpg")
	if got, want := state(), `Alter Text | ["https://example.test/a.jpg", "https://example.test/b.jpg"] |  178500/199900,  178500/199900`; got != want {
		t.Errorf("after import (only images are automatic):\n got  %s\n want %s", got, want)
	}
	approve("description")
	approve("ean")
	approve("rrp_cents") // RRP below the sales price: no compare-at price
	if got, want := state(), `Neuer Text | ["https://example.test/a.jpg", "https://example.test/b.jpg"] | 4006381333931 178500/-,  178500/199900`; got != want {
		t.Errorf("after approvals:\n got  %s\n want %s", got, want)
	}
	got := query(t, pool, `SELECT string_agg(entity_type || '.' || field || ' ' || origin || ' ' || coalesce(actor, '-'), ' | ' ORDER BY entity_type, field)
		FROM field_change WHERE reason = 'Änderung des Lieferanten übernommen'`)
	want := "product.description_html user robert | product.image_urls rule - | variant.compare_at_cents user robert | variant.ean user robert"
	if got != want {
		t.Errorf("field changes:\n got  %s\n want %s", got, want)
	}
}

func TestUsagePolicyKeepsSupplierDataOutOfProducts(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO usage_policy (source_id, brand, field_group, policy) VALUES
		  ('example', '', 'images', 'internal_only'),
		  ('example', 'Beispielrad', 'texts', 'ignore');
	`); err != nil {
		t.Fatal(err)
	}
	conn, err := csvfile.New(csvfile.Config{SourceID: "example", Delimiter: ";", DecimalComma: true,
		Columns: map[string]string{csvfile.SKU: "Nr", csvfile.Brand: "Marke", csvfile.Name: "Name", csvfile.ModelYear: "Jahr",
			csvfile.Color: "Farbe", csvfile.FrameSize: "RH", csvfile.Cost: "EK", csvfile.Description: "Text", csvfile.Images: "Bilder"}})
	if err != nil {
		t.Fatal(err)
	}
	load := func(row string) importer.Result {
		t.Helper()
		file := strings.NewReader("Nr;Marke;Name;Jahr;Farbe;RH;EK;Text;Bilder\n" + row + "\n")
		res, err := importer.Run(ctx, pool, conn, file, importer.Options{RawDir: t.TempDir(), MaxDropPct: 100})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	product := func() string {
		return query(t, pool, `SELECT '[' || description_html || '] ' || image_urls::text FROM product`)
	}

	load("1;Beispielrad;City 3;2027;rot;50;500,00;Text des Lieferanten;https://example.test/a.jpg")
	if err := tasks.Decide(ctx, pool, openModelTask(t, pool, "Beispielrad City 3 (2027)"), tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
		t.Fatal(err)
	}
	// Neither text nor images were copied; the supplier view has both.
	if got := product(); got != "[] []" {
		t.Errorf("product after approval: %s", got)
	}
	if got := query(t, pool, `SELECT description || ' ' || image_urls::text FROM supplier_item`); got != `Text des Lieferanten ["https://example.test/a.jpg"]` {
		t.Errorf("supplier view: %s", got)
	}

	// Text change: ignored, so no action and no task, but the supplier view
	// follows. Image change: a task appears, yet approving copies nothing.
	res := load("1;Beispielrad;City 3;2027;rot;50;500,00;Neuer Text;https://example.test/b.jpg")
	if res.Ignored != 1 || res.PendingApproval != 1 {
		t.Fatalf("run: %+v", res)
	}
	if n := query(t, pool, `SELECT count(*)::text FROM action WHERE field = 'description'`); n != "0" {
		t.Errorf("ignored text change created %s actions", n)
	}
	if got := query(t, pool, `SELECT description FROM supplier_item`); got != "Neuer Text" {
		t.Errorf("supplier view was not updated: %s", got)
	}
	var taskID int64
	if err := pool.QueryRow(ctx, `SELECT t.id FROM task t JOIN action a ON a.id = t.action_id WHERE a.field = 'image_urls' AND t.status = 'open'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
		t.Fatal(err)
	}
	if got := product(); got != "[] []" {
		t.Errorf("internal-only images reached the product: %s", got)
	}
}
