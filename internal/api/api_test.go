package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/dbtest"
	"velopim/internal/shopify"
)

// seed creates one approval task with action and item, and one plain task.
func seed(t *testing.T, pool *pgxpool.Pool) (approvalTask, plainTask int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO import_run (source_id, status, record_count) VALUES ('example', 'done', 1);
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year) VALUES ('example', '100-001', 'Beispielrad', 'Trekking 5.4', 2027);
		INSERT INTO rule (name, action_type, outcome) VALUES ('prices need approval', 'price_change', 'approval');
		INSERT INTO action (type, status, supplier_item_id, field, old_value, new_value, rule_id, decision_reason, base_price_cents, proposed_price_cents)
		  VALUES ('price_change', 'pending_approval', 1, 'cost_cents', '100000', '95000', 1, 'most specific rule matched', 178500, 173500);
	`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO task (kind, title, priority, action_id, supplier_item_id)
		VALUES ('approve_action', 'Preisänderung prüfen', 1, 1, 1) RETURNING id`).Scan(&approvalTask); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO task (kind, title, priority, import_run_id)
		VALUES ('data_issue', '1 Datenprobleme im Import', 3, 1) RETURNING id`).Scan(&plainTask); err != nil {
		t.Fatal(err)
	}
	return approvalTask, plainTask
}

func do(t *testing.T, h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getTasks(t *testing.T, h http.Handler, status string) TaskList {
	t.Helper()
	rec := do(t, h, "GET", "/api/tasks?status="+status, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET tasks %s: %d %s", status, rec.Code, rec.Body)
	}
	var list TaskList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestTaskListAndDecision(t *testing.T) {
	pool := dbtest.New(t)
	approval, plain := seed(t, pool)
	h := New(pool, "tester", nil)
	ctx := context.Background()

	list := getTasks(t, h, "open")
	if list.Open != 2 || list.Closed != 0 || len(list.Tasks) != 2 {
		t.Fatalf("open list: %+v", list)
	}
	first := list.Tasks[0] // priority 1 comes first
	if first.ID != approval || first.Action == nil || first.Item == nil {
		t.Fatalf("first task: %+v", first)
	}
	if a := first.Action; a.Type != "price_change" || a.Field != "cost_cents" || a.Old != "100000" || a.New != "95000" ||
		a.Rule == nil || *a.Rule != "prices need approval" || a.ProposedPriceCents == nil || *a.ProposedPriceCents != 173500 ||
		a.BasePriceCents == nil || *a.BasePriceCents != 178500 {
		t.Errorf("action: %+v", a)
	}
	if it := first.Item; it.SKU != "100-001" || it.Brand != "Beispielrad" || it.ModelYear == nil || *it.ModelYear != 2027 {
		t.Errorf("item: %+v", it)
	}
	if second := list.Tasks[1]; second.ID != plain || second.Action != nil || second.Item != nil {
		t.Errorf("plain task: %+v", second)
	}

	path := "/api/tasks/1/decision"
	cases := []struct {
		name, path, contentType, body string
		want                          int
	}{
		{"form post is refused", path, "application/x-www-form-urlencoded", "approve=true", http.StatusUnsupportedMediaType},
		{"no content type", path, "", `{"approve":true}`, http.StatusUnsupportedMediaType},
		{"broken json", path, "application/json", `{`, http.StatusBadRequest},
		{"unknown field", path, "application/json", `{"approve":true,"actor":"x"}`, http.StatusBadRequest},
		{"bad id", "/api/tasks/abc/decision", "application/json", `{"approve":true}`, http.StatusBadRequest},
		{"unknown task", "/api/tasks/999/decision", "application/json", `{"approve":true}`, http.StatusConflict},
		{"approve", path, "application/json; charset=utf-8", `{"approve":true,"note":"passt"}`, http.StatusNoContent},
		{"already decided", path, "application/json", `{"approve":false}`, http.StatusConflict},
	}
	for _, c := range cases {
		if rec := do(t, h, "POST", c.path, c.contentType, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}

	var status, by, note string
	if err := pool.QueryRow(ctx, `SELECT status, decided_by, decision_note FROM action WHERE id = 1`).Scan(&status, &by, &note); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || by != "tester" || note != "passt" {
		t.Errorf("action after approval: %s by %s (%s)", status, by, note)
	}

	// Rejecting a task without action just dismisses it.
	if rec := do(t, h, "POST", "/api/tasks/2/decision", "application/json", `{"approve":false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss: %d %s", rec.Code, rec.Body)
	}
	if open := getTasks(t, h, "open"); open.Open != 0 || len(open.Tasks) != 0 || open.Closed != 2 {
		t.Errorf("open list after decisions: %+v", open)
	}
	closed := getTasks(t, h, "closed")
	if len(closed.Tasks) != 2 {
		t.Fatalf("closed list: %+v", closed)
	}
	for _, task := range closed.Tasks {
		if task.ClosedAt == nil || task.ClosedBy == nil || *task.ClosedBy != "tester" {
			t.Errorf("closed task %d lacks who and when: %+v", task.ID, task)
		}
	}

	if rec := do(t, h, "GET", "/api/tasks?status=bogus", "", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bogus status: %d", rec.Code)
	}
}

func TestModelTaskListsItsVariants(t *testing.T) {
	pool := dbtest.New(t)
	seed(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year, color, frame_size) VALUES
		  ('example', '200-002', 'Beispielrad', 'City 3', 2027, 'rot', '54'),
		  ('example', '200-001', 'Beispielrad', 'City 3', 2027, 'rot', '50'),
		  ('example', '200-003', 'Beispielrad', 'City 3', 2027, 'blau', '50');
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'k');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 2, id FROM supplier_item WHERE name = 'City 3';
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen', 2, 2);
	`); err != nil {
		t.Fatal(err)
	}
	list := getTasks(t, New(pool, "tester", nil), "open")
	var skus []string
	for _, task := range list.Tasks {
		if task.Variants == nil {
			t.Errorf("task %d: variants must be an empty list, not null", task.ID)
		}
		if task.Title == "Neues Modell prüfen" {
			if task.Item != nil || task.Action == nil || task.Action.New != "Beispielrad City 3 (2027)" {
				t.Errorf("model task: %+v", task)
			}
			for _, v := range task.Variants {
				skus = append(skus, v.SKU)
			}
		} else if len(task.Variants) != 0 {
			t.Errorf("task %d has variants: %+v", task.ID, task.Variants)
		}
	}
	// Sorted by color, then frame size.
	if got := strings.Join(skus, ","); got != "200-003,200-001,200-002" {
		t.Errorf("variants = %s", got)
	}
}

func TestImportRunsAndFallbacks(t *testing.T) {
	pool := dbtest.New(t)
	seed(t, pool)
	h := New(pool, "tester", nil)

	rec := do(t, h, "GET", "/api/import-runs", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("import runs: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Runs []ImportRun `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Runs) != 1 || body.Runs[0].SourceName != "Beispiel" || body.Runs[0].Status != "done" {
		t.Errorf("runs: %+v", body.Runs)
	}

	if rec := do(t, h, "GET", "/api/health", "", ""); rec.Code != http.StatusOK {
		t.Errorf("health: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/nope", "", ""); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("unknown path: %d %s", rec.Code, rec.Body)
	}
}

func TestPricingRulesAndModelApproval(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year, color, frame_size, cost_cents, rrp_cents, attributes) VALUES
		  ('example', '200-001', 'Beispielrad', 'City 3', 2027, 'rot', '50', 50000, 89900, '{"Rahmenform": "Wave"}'),
		  ('example', '200-002', 'Beispielrad', 'City 3', 2027, 'rot', '50', 50000, 89900, '{"Rahmenform": "Herren"}');
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'm');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 1, id FROM supplier_item;
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen', 2, 1);
	`); err != nil {
		t.Fatal(err)
	}
	const jsonType = "application/json"

	// Pricing rules: invalid ones are refused, a second PUT for the same
	// scope replaces the first.
	cases := []struct {
		name, body string
		want       int
	}{
		{"unknown mode", `{"brand":"","category":"","config":{"mode":"magic","value":1.5}}`, http.StatusUnprocessableEntity},
		{"factor zero", `{"brand":"","category":"","config":{"mode":"markup","value":0}}`, http.StatusUnprocessableEntity},
		{"unknown rounding", `{"brand":"","category":"","config":{"mode":"markup","value":1.5,"rounding":"up"}}`, http.StatusUnprocessableEntity},
		{"global rule", `{"brand":"","category":"","config":{"mode":"markup","value":2,"vat_pct":19,"rounding":"none"}}`, http.StatusOK},
		{"replace it", `{"brand":"","category":"","config":{"mode":"markup","value":1.5,"vat_pct":19,"rounding":"end_99","cap_at_rrp":true}}`, http.StatusOK},
	}
	for _, c := range cases {
		if rec := do(t, h, "PUT", "/api/pricing-rules", jsonType, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
	var rules struct {
		Rules []PricingRule `json:"rules"`
	}
	rec := do(t, h, "GET", "/api/pricing-rules", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 1 || rules.Rules[0].Config.Value != 1.5 || !rules.Rules[0].Config.CapAtRRP {
		t.Fatalf("rules: %+v", rules.Rules)
	}

	// The task shows purchase price, RRP and the calculated price:
	// 500 € × 1.5 × 1.19 = 892.50 €, rounded to 892.99 €, below the RRP.
	list := getTasks(t, h, "open")
	if len(list.Tasks) != 1 || len(list.Tasks[0].Variants) != 2 || list.Tasks[0].LockedSplitBy != nil {
		t.Fatalf("tasks: %+v", list.Tasks)
	}
	v := list.Tasks[0].Variants[0]
	if v.CostCents == nil || *v.CostCents != 50000 || v.RRPCents == nil || *v.RRPCents != 89900 ||
		v.PriceCents == nil || *v.PriceCents != 89299 || v.Attributes["Rahmenform"] == "" {
		t.Errorf("variant in task: %+v", v)
	}

	// Same color and frame size: approval is refused until the model is
	// split by the attribute that tells the articles apart.
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType, `{"approve":true}`); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "nicht unterscheidbar") {
		t.Fatalf("blocked approval: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType, `{"approve":true,"split_by":["attributes.Rahmenform"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approval with split: %d %s", rec.Code, rec.Body)
	}

	var products struct {
		Products []Product `json:"products"`
	}
	rec = do(t, h, "GET", "/api/products", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &products); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range products.Products {
		for _, v := range p.Variants {
			got = append(got, p.Name+" "+p.Status+" "+v.SKU+" "+*v.SupplierSKU+" "+strings.Join(v.OptionValues, "/"))
			if v.PriceCents == nil || *v.PriceCents != 89299 || v.CompareAtCents == nil || *v.CompareAtCents != 89900 {
				t.Errorf("%s: price %v, compare-at %v", v.SKU, v.PriceCents, v.CompareAtCents)
			}
		}
	}
	want := "City 3 Herren draft 10001 200-002 rot/50, City 3 Wave draft 10002 200-001 rot/50"
	if s := strings.Join(got, ", "); s != want {
		t.Errorf("products:\n got  %s\n want %s", s, want)
	}

	// Deleting a rule.
	path := "/api/pricing-rules/" + strconv.FormatInt(rules.Rules[0].ID, 10)
	if rec := do(t, h, "DELETE", path, "", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", path, "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("second delete: %d", rec.Code)
	}
}

func TestSearchAndFilters(t *testing.T) {
	pool := dbtest.New(t)
	approval, _ := seed(t, pool) // one price task for article 100-001, one data issue task
	h := New(pool, "tester", nil)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO supplier_item (source_id, sku, ean, brand, name, model_year, color, frame_size, cost_cents) VALUES
		  ('example', '200-001', '4006381333931', 'Beispielrad', 'City 3', 2027, 'rot', '50', 50000),
		  ('example', '200-002', '', 'Beispielrad', 'City 3', 2027, 'blau', '50', 50000);
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'm');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 2, id FROM supplier_item WHERE name = 'City 3';
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen: Beispielrad City 3 (2027), 2 Varianten', 2, 2);
	`); err != nil {
		t.Fatal(err)
	}
	titles := func(query string) string {
		t.Helper()
		rec := do(t, h, "GET", "/api/tasks?"+query, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks?%s: %d %s", query, rec.Code, rec.Body)
		}
		var list TaskList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, task := range list.Tasks {
			out = append(out, task.Category)
		}
		return strings.Join(out, ",")
	}
	cases := []struct{ query, want string }{
		{"status=open", "price,new_model,import"},
		{"category=price", "price"},
		{"category=import", "import"},
		{"q=city", "new_model"},
		{"q=100-001", "price"},     // article of the task
		{"q=200-002", "new_model"}, // article inside a model task
		{"q=4006381333931", "new_model"},
		{"q=%25", ""}, // a percent sign is text, not a wildcard
		{"q=city&category=price", ""},
		{"status=closed", ""},
	}
	for _, c := range cases {
		if got := titles(c.query); got != c.want {
			t.Errorf("tasks?%s = %q, want %q", c.query, got, c.want)
		}
	}
	list := getTasks(t, h, "open")
	if list.Categories["price"] != 1 || list.Categories["new_model"] != 1 || list.Categories["import"] != 1 || len(list.Categories) != 3 {
		t.Errorf("categories: %v", list.Categories)
	}
	if rec := do(t, h, "GET", "/api/tasks?category=bogus", "", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bogus category: %d", rec.Code)
	}

	// Approve the price task, then the model split by color: two products
	// of one model, one variant without price (no pricing rule).
	if rec := do(t, h, "POST", "/api/tasks/"+strconv.FormatInt(approval, 10)+"/decision", "application/json", `{"approve":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approve price: %d %s", rec.Code, rec.Body)
	}
	if got := titles("status=closed&q=100-001"); got != "price" {
		t.Errorf("closed search = %q", got)
	}
	if rec := do(t, h, "POST", "/api/tasks/3/decision", "application/json",
		`{"approve":true,"split_by":["color"],"variant_prices":{"200-001":89900}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approve model: %d %s", rec.Code, rec.Body)
	}
	products := func(query string) (string, ProductList) {
		t.Helper()
		rec := do(t, h, "GET", "/api/products?"+query, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET products?%s: %d %s", query, rec.Code, rec.Body)
		}
		var list ProductList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range list.Products {
			out = append(out, p.Name)
		}
		return strings.Join(out, ","), list
	}
	for _, c := range []struct{ query, want string }{
		{"", "City 3 blau,City 3 rot"},
		{"unpriced=true", "City 3 blau"},
		{"q=200-001", "City 3 rot"}, // supplier article number
		{"q=10002", "City 3 rot"},   // own article number
		{"q=beispielrad", "City 3 blau,City 3 rot"},
		{"status=active", ""},
		{"status=draft&q=blau", "City 3 blau"},
	} {
		if got, _ := products(c.query); got != c.want {
			t.Errorf("products?%s = %q, want %q", c.query, got, c.want)
		}
	}
	_, all := products("")
	if all.Total != 2 || all.Unpriced != 1 {
		t.Errorf("counts: total %d, unpriced %d", all.Total, all.Unpriced)
	}
	for _, p := range all.Products {
		if p.ModelKey != "m" || p.ModelName != "City 3" || len(p.Split) != 1 || p.Split[0].Axis != "color" {
			t.Errorf("model of %s: key %q name %q split %v", p.Name, p.ModelKey, p.ModelName, p.Split)
		}
	}

	// Supplier articles: found by EAN, with the own article number once a
	// variant exists; an empty query finds nothing.
	var items struct {
		Items []SupplierItem `json:"items"`
	}
	rec := do(t, h, "GET", "/api/supplier-items?q=4006381333931", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items.Items) != 1 || items.Items[0].SKU != "200-001" || items.Items[0].OwnSKU == nil || *items.Items[0].OwnSKU != "10002" {
		t.Errorf("supplier item search: %+v", items.Items)
	}
	rec = do(t, h, "GET", "/api/supplier-items?q=", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items.Items) != 0 {
		t.Errorf("empty search: %v %+v", err, items.Items)
	}
}

func TestRules(t *testing.T) {
	pool := dbtest.New(t)
	seed(t, pool) // rule 1 "prices need approval" has decided an action
	h := New(pool, "tester", nil)
	const jsonType = "application/json"

	create := `{"name":"Senkungen weitergeben","enabled":true,"action_type":"price_change","scope":{"source_id":"example","brand":"Beispielrad"},
		"conditions":[{"fact":"cost_change_cents","op":"lt","value":0}],"outcome":"auto","guard":false}`
	rec := do(t, h, "POST", "/api/rules", jsonType, create)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	for name, body := range map[string]string{
		"no name":         `{"name":"","action_type":"price_change","outcome":"auto"}`,
		"automatic guard": `{"name":"x","action_type":"price_change","outcome":"auto","guard":true}`,
		"unknown fact":    `{"name":"x","action_type":"price_change","outcome":"auto","conditions":[{"fact":"moon","op":"lt","value":1}]}`,
	} {
		if rec := do(t, h, "POST", "/api/rules", jsonType, body); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", name, rec.Code)
		}
	}

	var list RuleList
	rec = do(t, h, "GET", "/api/rules", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Rules) != 2 || len(list.Sources) != 1 || list.Sources[0].ID != "example" {
		t.Fatalf("list: %+v", list)
	}
	var created int64
	for _, r := range list.Rules {
		if r.Name == "Senkungen weitergeben" {
			created = r.ID
			if !r.Enabled || r.Scope.Brand != "Beispielrad" || len(r.Conditions) != 1 || r.Conditions[0].Fact != "cost_change_cents" {
				t.Errorf("created rule: %+v", r)
			}
		}
	}
	path := "/api/rules/" + strconv.FormatInt(created, 10)

	// Disable it, then delete it. The seeded rule has decided an action and
	// can only be disabled.
	if rec := do(t, h, "PUT", path, jsonType, strings.Replace(create, `"enabled":true`, `"enabled":false`, 1)); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	var enabled bool
	if err := pool.QueryRow(context.Background(), `SELECT enabled FROM rule WHERE id = $1`, created).Scan(&enabled); err != nil || enabled {
		t.Errorf("rule still enabled (%v)", err)
	}
	if rec := do(t, h, "DELETE", path, "", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do(t, h, "PUT", path, jsonType, create); rec.Code != http.StatusNotFound {
		t.Errorf("update of a deleted rule: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/rules/1", "", ""); rec.Code != http.StatusConflict {
		t.Errorf("delete of a used rule: %d, want 409", rec.Code)
	}
}

func TestProductDetailAndEditing(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil)
	const jsonType = "application/json"
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year, color, frame_size, cost_cents, rrp_cents) VALUES
		  ('example', '200-001', 'Beispielrad', 'City 3', 2027, 'rot', '50', 50000, 89900),
		  ('example', '200-002', 'Beispielrad', 'City 3', 2027, 'blau', '50', 50000, 89900);
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'm');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 1, id FROM supplier_item;
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen', 2, 1);
	`); err != nil {
		t.Fatal(err)
	}
	// Split by color; only the red variant gets a price.
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType,
		`{"approve":true,"split_by":["color"],"variant_prices":{"200-001":89900}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	detail := func(id string) ProductDetail {
		t.Helper()
		rec := do(t, h, "GET", "/api/products/"+id, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET product %s: %d %s", id, rec.Code, rec.Body)
		}
		var d ProductDetail
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}

	// Asking for one product returns both products of the model.
	d := detail("1")
	if len(d.Products) != 2 || d.Products[0].Name != "City 3 blau" || d.Products[1].Name != "City 3 rot" || len(d.Notes) != 0 {
		t.Fatalf("detail: %+v", d)
	}
	blue, red := d.Products[0], d.Products[1]

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"empty name", "PATCH", "/api/products/1", `{"name":" "}`, http.StatusUnprocessableEntity},
		{"unknown product", "PATCH", "/api/products/99", `{"name":"x"}`, http.StatusNotFound},
		{"rename and describe", "PATCH", "/api/products/1", `{"name":"City 3 Blau","description_html":"<p>Neu</p>"}`, http.StatusNoContent},
		{"into the store without price", "PATCH", "/api/products/" + strconv.FormatInt(blue.ID, 10), `{"wanted":true}`, http.StatusUnprocessableEntity},
		{"into the store with price", "PATCH", "/api/products/" + strconv.FormatInt(red.ID, 10), `{"wanted":true}`, http.StatusNoContent},
		{"price of zero", "PATCH", "/api/variants/" + strconv.FormatInt(blue.Variants[0].ID, 10), `{"price_cents":0}`, http.StatusUnprocessableEntity},
		{"set a price", "PATCH", "/api/variants/" + strconv.FormatInt(blue.Variants[0].ID, 10), `{"price_cents":84900}`, http.StatusNoContent},
		{"unknown variant", "PATCH", "/api/variants/99", `{"price_cents":100}`, http.StatusNotFound},
		{"empty note", "POST", "/api/products/1/notes", `{"body":"  "}`, http.StatusUnprocessableEntity},
		{"note", "POST", "/api/products/1/notes", `{"body":"Ansprechpartner: Herr Beispiel"}`, http.StatusNoContent},
	}
	for _, c := range cases {
		if rec := do(t, h, c.method, c.path, jsonType, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}

	d = detail("2")
	blue, red = d.Products[0], d.Products[1]
	if blue.Name != "City 3 Blau" || blue.Description != "<p>Neu</p>" || blue.Wanted || !red.Wanted ||
		blue.Variants[0].PriceCents == nil || *blue.Variants[0].PriceCents != 84900 {
		t.Errorf("after editing: blue %+v, red wanted %v", blue, red.Wanted)
	}
	if len(d.Notes) != 1 || d.Notes[0].Author != "tester" || d.Notes[0].Body != "Ansprechpartner: Herr Beispiel" {
		t.Errorf("notes: %+v", d.Notes)
	}
	// The history says who changed what by hand.
	var manual []string
	for _, c := range d.History {
		if c.Origin == "user" && c.Reason == nil || c.Reason != nil && *c.Reason == "Preis von Hand geändert" {
			manual = append(manual, c.Field+"="+*c.New+" by "+*c.Actor)
		}
	}
	want := "price_cents=84900 by tester, in_store=true by tester, description_html=<p>Neu</p> by tester, name=City 3 Blau by tester"
	if got := strings.Join(manual, ", "); got != want {
		t.Errorf("manual changes:\n got  %s\n want %s", got, want)
	}
	if rec := do(t, h, "GET", "/api/products/99", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown product: %d", rec.Code)
	}
}

func TestUsagePolicies(t *testing.T) {
	pool := dbtest.New(t)
	seed(t, pool)
	h := New(pool, "tester", nil)
	const jsonType = "application/json"
	cases := []struct {
		name, body string
		want       int
	}{
		{"unknown group", `{"source_id":"example","brand":"","field_group":"colors","policy":"use"}`, http.StatusUnprocessableEntity},
		{"unknown policy", `{"source_id":"example","brand":"","field_group":"images","policy":"maybe"}`, http.StatusUnprocessableEntity},
		{"unknown source", `{"source_id":"nobody","brand":"","field_group":"images","policy":"ignore"}`, http.StatusUnprocessableEntity},
		{"images internal for the source", `{"source_id":"example","brand":"","field_group":"images","policy":"internal_only"}`, http.StatusNoContent},
		{"but usable for one brand", `{"source_id":"example","brand":"Beispielrad","field_group":"images","policy":"use"}`, http.StatusNoContent},
		{"texts ignored", `{"source_id":"example","brand":"","field_group":"texts","policy":"ignore"}`, http.StatusNoContent},
		{"texts back to the default", `{"source_id":"example","brand":"","field_group":"texts","policy":"use"}`, http.StatusNoContent},
	}
	for _, c := range cases {
		if rec := do(t, h, "PUT", "/api/usage-policies", jsonType, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
	list := func() string {
		t.Helper()
		var out UsagePolicyList
		rec := do(t, h, "GET", "/api/usage-policies", "", "")
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, p := range out.Policies {
			parts = append(parts, p.SourceID+"/"+p.Brand+"/"+p.Group+"="+p.Policy)
		}
		return strings.Join(parts, " ")
	}
	if got, want := list(), "example//images=internal_only example/Beispielrad/images=use"; got != want {
		t.Errorf("policies = %q, want %q", got, want)
	}
	if rec := do(t, h, "DELETE", "/api/usage-policies?source_id=example&brand=Beispielrad&field_group=images", "", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/usage-policies?source_id=example&brand=Beispielrad&field_group=images", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("second delete: %d", rec.Code)
	}
	if got, want := list(), "example//images=internal_only"; got != want {
		t.Errorf("policies = %q, want %q", got, want)
	}
}

func TestSyncPlanIsADryRun(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, cost_cents, availability, availability_status) VALUES
		  ('example', '200-001', 'Beispielrad', 'City 3', 50000, '{"status": "in_stock"}', 'in_stock'),
		  ('example', '200-002', 'Beispielrad', 'City 3', 50000, '{"status": "unavailable"}', 'unavailable');
		INSERT INTO product (brand, name, category, description_html, option_names) VALUES
		  ('Beispielrad', 'City 3', 'Citybike', '<p>Text</p>', '["Rahmenhöhe"]'),
		  ('Beispielrad', 'Nicht gewählt', '', '', '[]');
		INSERT INTO variant (product_id, sku, option_values, price_cents) VALUES (1, '10001', '["50"]', 89900), (1, '10002', '["54"]', 89900);
		INSERT INTO variant_supplier_item (variant_id, supplier_item_id, preferred) VALUES (1, 1, true), (2, 2, true);
		INSERT INTO shopify_listing (product_id, wanted) VALUES (1, true), (2, false);
	`); err != nil {
		t.Fatal(err)
	}
	rec := do(t, h, "GET", "/api/sync-plan", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("sync plan: %d %s", rec.Code, rec.Body)
	}
	var plan SyncPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Products) != 1 {
		t.Fatalf("only wanted products are planned: %+v", plan.Products)
	}
	p := plan.Products[0]
	if p.Name != "Beispielrad City 3" || p.InShopify || len(p.Ops) != 1 || p.Ops[0].Kind != "create_product" || len(p.Issues) != 0 {
		t.Errorf("plan: %+v", p)
	}
	proj := p.Projection
	if proj.Published || proj.Vendor != "Beispielrad" || proj.ProductType != "Citybike" || len(proj.Variants) != 2 ||
		!proj.Variants[0].Sellable || proj.Variants[1].Sellable || proj.Variants[1].DeliveryText != "Derzeit nicht lieferbar" {
		t.Errorf("projection: %+v", proj)
	}
	// The purchase price is internal and must not appear anywhere in the plan.
	if strings.Contains(rec.Body.String(), "50000") || strings.Contains(rec.Body.String(), "cost") {
		t.Errorf("plan leaks the purchase price: %s", rec.Body)
	}
	// Planning writes nothing.
	var synced int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM shopify_listing WHERE synced_projection IS NOT NULL OR shopify_product_id IS NOT NULL`).Scan(&synced); err != nil || synced != 0 {
		t.Errorf("dry run changed the listing (%d, %v)", synced, err)
	}
}

// planStore is a Shopify stand-in for the sync endpoint.
type planStore struct {
	sent int
	err  error
}

func (s *planStore) WaitForMedia(context.Context, string, time.Duration) ([]shopify.MediaState, error) {
	return nil, nil
}

func (s *planStore) SetProduct(_ context.Context, p shopify.Product, id string, _ map[string]string) (shopify.SetResult, error) {
	if s.err != nil {
		return shopify.SetResult{}, s.err
	}
	s.sent++
	return shopify.SetResult{ProductID: "gid://shopify/Product/7", NumericID: "7", VariantIDs: map[string]string{}}, nil
}

func (s *planStore) AdminURL(id string) string { return "https://admin.example/products/" + id }

func TestSyncEndpoint(t *testing.T) {
	pool := dbtest.New(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO product (brand, name) VALUES ('Beispielrad', 'City 3');
		INSERT INTO variant (product_id, sku, price_cents) VALUES (1, '10001', 89900);
		INSERT INTO shopify_listing (product_id, wanted) VALUES (1, true);
	`); err != nil {
		t.Fatal(err)
	}
	const jsonType = "application/json"

	// No store configured.
	if rec := do(t, New(pool, "tester", nil), "POST", "/api/sync", jsonType, `{}`); rec.Code != http.StatusConflict {
		t.Errorf("without store: %d, want 409", rec.Code)
	}
	// Writing switched off: 409, nothing recorded against the product.
	off := &planStore{err: shopify.ErrWritesDisabled}
	if rec := do(t, New(pool, "tester", off), "POST", "/api/sync", jsonType, `{}`); rec.Code != http.StatusConflict {
		t.Errorf("writing off: %d, want 409", rec.Code)
	}
	// A cross-site form post cannot trigger a sync.
	store := &planStore{}
	h := New(pool, "tester", store)
	if rec := do(t, h, "POST", "/api/sync", "application/x-www-form-urlencoded", ""); rec.Code != http.StatusUnsupportedMediaType || store.sent != 0 {
		t.Errorf("form post: %d, sent %d", rec.Code, store.sent)
	}

	rec := do(t, h, "POST", "/api/sync", jsonType, `{}`)
	if rec.Code != http.StatusOK || store.sent != 1 || !strings.Contains(rec.Body.String(), `"synced":1`) || !strings.Contains(rec.Body.String(), `"created":true`) {
		t.Fatalf("sync: %d %s (sent %d)", rec.Code, rec.Body, store.sent)
	}
	// The new draft shows up as a publish task in the Shopify category.
	list := getTasks(t, h, "open")
	if len(list.Tasks) != 1 || list.Tasks[0].Category != "shopify" || list.Tasks[0].Kind != "publish_product" ||
		list.Tasks[0].ProductID == nil || !strings.Contains(list.Tasks[0].Detail, "https://admin.example/products/7") {
		t.Fatalf("tasks after sync: %+v", list.Tasks)
	}
	// Approving it makes the product active.
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType, `{"approve":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM product WHERE id = 1`).Scan(&status); err != nil || status != "active" {
		t.Errorf("product status = %s (%v)", status, err)
	}
	rec = do(t, h, "POST", "/api/sync", jsonType, `{}`)
	if store.sent != 2 || !strings.Contains(rec.Body.String(), `"synced":1`) {
		t.Errorf("publishing sync: sent %d, %s", store.sent, rec.Body)
	}
}

func TestUploadImport(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil, WithRawDir(t.TempDir()))
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind, connector_config) VALUES
		  ('example', 'Beispiel', 'csvfile', '{"delimiter": ";", "decimal_comma": true, "columns": {"sku": "Nr", "brand": "Marke", "name": "Name", "cost": "EK"}}'),
		  ('broken', 'Kaputt', 'csvfile', '{}');
	`); err != nil {
		t.Fatal(err)
	}
	file := "Nr;Marke;Name;EK\n1;Beispielrad;City 3;500,00\n2;Beispielrad;City 3;500,00\n"
	const octet = "application/octet-stream"

	cases := []struct {
		name, path, contentType, body string
		want                          int
	}{
		{"form upload from another site", "/api/sources/example/imports", "multipart/form-data; boundary=x", file, http.StatusUnsupportedMediaType},
		{"unknown source", "/api/sources/nobody/imports", octet, file, http.StatusNotFound},
		{"source without usable configuration", "/api/sources/broken/imports", octet, file, http.StatusNotFound},
		{"file without the article number column", "/api/sources/example/imports", octet, "A;B\n1;2\n", http.StatusUnprocessableEntity},
		{"valid file", "/api/sources/example/imports", octet, file, http.StatusOK},
	}
	var last *httptest.ResponseRecorder
	for _, c := range cases {
		last = do(t, h, "POST", c.path, c.contentType, c.body)
		if last.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, last.Code, c.want, last.Body)
		}
	}
	if body := last.Body.String(); !strings.Contains(body, `"new":2`) || !strings.Contains(body, `"pending_approval":1`) || !strings.Contains(body, `"status":"done"`) {
		t.Errorf("import result: %s", body)
	}
	// The failed attempts left no import run behind; the model task exists.
	var runs, tasks int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM import_run), (SELECT count(*) FROM task WHERE status = 'open')`).Scan(&runs, &tasks); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || tasks < 1 {
		t.Errorf("runs = %d, open tasks = %d", runs, tasks)
	}
	// The import list names the sources for the upload form.
	if rec := do(t, h, "GET", "/api/import-runs", "", ""); !strings.Contains(rec.Body.String(), `"sources":[`) || !strings.Contains(rec.Body.String(), `"Beispiel"`) {
		t.Errorf("import list: %s", rec.Body)
	}
}

func TestResplit(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil)
	const jsonType = "application/json"
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year, color, frame_size, cost_cents, attributes, image_urls) VALUES
		  ('example', 'A', 'Beispielrad', 'City 3', 2027, 'rot', '50', 50000, '{"Rahmenform": "Wave"}', '["https://img.test/rot.jpg"]'),
		  ('example', 'B', 'Beispielrad', 'City 3', 2027, 'rot', '54', 50000, '{"Rahmenform": "Wave"}', '["https://img.test/rot.jpg"]'),
		  ('example', 'C', 'Beispielrad', 'City 3', 2027, 'blau', '50', 50000, '{"Rahmenform": "Herren"}', '["https://img.test/blau.jpg"]');
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'm');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 1, id FROM supplier_item;
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen', 2, 1);
	`); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType,
		`{"approve":true,"variant_prices":{"A":89900,"B":89900,"C":84900}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	// One product with three variants; a note and a hand-written text.
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/products/1/notes", `{"body":"Bestellhinweis"}`},
		{"PATCH", "/api/products/1", `{"description_html":"<p>Eigener Text</p>"}`},
	} {
		if rec := do(t, h, c.method, c.path, jsonType, c.body); rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	get := func(id string) ProductDetail {
		t.Helper()
		var d ProductDetail
		rec := do(t, h, "GET", "/api/products/"+id, "", "")
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("GET product %s: %d %v", id, rec.Code, err)
		}
		return d
	}
	summary := func(d ProductDetail) string {
		var parts []string
		for _, p := range d.Products {
			s := p.Name + " [" + strings.Join(p.OptionNames, ",") + "]"
			for _, v := range p.Variants {
				s += " " + v.SKU + "=" + strings.Join(v.OptionValues, "/") + "@" + strconv.FormatInt(*v.PriceCents, 10)
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, " | ")
	}

	before := get("1")
	if got := summary(before); got != "City 3 [Farbe,Rahmenhöhe] 10001=blau/50@84900 10002=rot/50@89900 10003=rot/54@89900" {
		t.Fatalf("before: %s", got)
	}
	if strings.Join(before.SplitAxes, ",") != "color,frame_size,attributes.Rahmenform" || len(before.SplitBy) != 0 || !before.CanResplit {
		t.Errorf("split info: axes %v, by %v, can %v", before.SplitAxes, before.SplitBy, before.CanResplit)
	}

	// Split by color: two products. Article numbers and prices stay.
	rec := do(t, h, "PUT", "/api/products/1/split", jsonType, `{"split_by":["color"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resplit: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		ProductID int64 `json:"product_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	after := get(strconv.FormatInt(out.ProductID, 10))
	if got := summary(after); got != "City 3 blau [Rahmenhöhe] 10001=50@84900 | City 3 rot [Rahmenhöhe] 10002=50@89900 10003=54@89900" {
		t.Errorf("after: %s", got)
	}
	// The hand-written text and the note survive; images follow the cut.
	if after.Products[0].Description != "<p>Eigener Text</p>" || len(after.Notes) != 1 ||
		strings.Join(after.Products[0].ImageURLs, ",") != "https://img.test/blau.jpg" || strings.Join(after.Products[1].ImageURLs, ",") != "https://img.test/rot.jpg" {
		t.Errorf("carried over: %+v notes %d", after.Products[0], len(after.Notes))
	}
	if strings.Join(after.SplitBy, ",") != "color" {
		t.Errorf("split_by after: %v", after.SplitBy)
	}
	if rec := do(t, h, "GET", "/api/products/1", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("old product still exists: %d", rec.Code)
	}

	// Back to one product, then a cut that cannot work.
	id := strconv.FormatInt(out.ProductID, 10)
	if rec := do(t, h, "PUT", "/api/products/"+id+"/split", jsonType, `{"split_by":["bogus"]}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown axis: %d", rec.Code)
	}
	// Once a product is in Shopify the cut is fixed.
	if _, err := pool.Exec(context.Background(), `INSERT INTO shopify_listing (product_id, wanted, shopify_product_id) VALUES ($1, true, 'gid://shopify/Product/1')`, out.ProductID); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, "PUT", "/api/products/"+id+"/split", jsonType, `{"split_by":[]}`); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "schon in Shopify") {
		t.Errorf("resplit of a synced model: %d %s", rec.Code, rec.Body)
	}
	if get(id).CanResplit {
		t.Error("can_resplit must be false once the model is in Shopify")
	}
}

func TestSources(t *testing.T) {
	pool := dbtest.New(t)
	h := New(pool, "tester", nil, WithRawDir(t.TempDir()))
	const jsonType = "application/json"
	valid := `{"name":"Hartje","connector_config":{"delimiter":";","decimal_comma":true,"encoding":"windows-1252",
		"columns":{"sku":"Artikelnummer","name":"Bezeichnung","brand":"Marke","cost":"EK"},"attribute_columns":["Antrieb"]},
		"availability_mapping":[{"name":"auf Lager","when":[{"field":"Bestand","op":"gt","value":"0"}],"status":"in_stock","quantity_field":"Bestand"},
		{"name":"sonst","when":[],"status":"unavailable"}]}`
	cases := []struct {
		name, id, body string
		want           int
	}{
		{"bad id", "Hartje GmbH", valid, http.StatusUnprocessableEntity},
		{"no name", "hartje", `{"name":"","connector_config":{"delimiter":";","columns":{"sku":"Nr"}}}`, http.StatusUnprocessableEntity},
		{"no article number column", "hartje", `{"name":"Hartje","connector_config":{"delimiter":";","columns":{"name":"Bezeichnung"}}}`, http.StatusUnprocessableEntity},
		{"delimiter too long", "hartje", `{"name":"Hartje","connector_config":{"delimiter":";;","columns":{"sku":"Nr"}}}`, http.StatusUnprocessableEntity},
		{"unknown availability status", "hartje", `{"name":"Hartje","connector_config":{"delimiter":";","columns":{"sku":"Nr"}},
			"availability_mapping":[{"name":"x","status":"bald"}]}`, http.StatusUnprocessableEntity},
		{"valid", "hartje", valid, http.StatusNoContent},
		{"change it", "hartje", strings.Replace(valid, `"name":"Hartje"`, `"name":"Hartje GmbH"`, 1), http.StatusNoContent},
	}
	for _, c := range cases {
		if rec := do(t, h, "PUT", "/api/sources/"+strings.ReplaceAll(c.id, " ", "%20"), jsonType, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
	var list struct {
		Sources []SourceConfig `json:"sources"`
	}
	rec := do(t, h, "GET", "/api/sources", "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Sources) != 1 {
		t.Fatalf("sources: %+v", list.Sources)
	}
	src := list.Sources[0]
	if src.ID != "hartje" || src.Name != "Hartje GmbH" || src.Config.Encoding != "windows-1252" || src.Config.Columns["sku"] != "Artikelnummer" ||
		len(src.AvailabilityMapping) != 2 || src.AvailabilityMapping[0].Status != "in_stock" {
		t.Errorf("stored source: %+v", src)
	}

	// The stored configuration imports a Windows-1252 file through the upload.
	file := "Artikelnummer;Bezeichnung;Marke;EK;Bestand;Antrieb\n1;Tourenrad Gr\xf6\xdfe L;Beispielrad;500,00;3;Kette\n"
	rec = do(t, h, "POST", "/api/sources/hartje/imports", "application/octet-stream", file)
	if rec.Code != http.StatusOK {
		t.Fatalf("import with the stored configuration: %d %s", rec.Code, rec.Body)
	}
	var name, status string
	if err := pool.QueryRow(context.Background(), `SELECT name, availability_status FROM supplier_item WHERE sku = '1'`).Scan(&name, &status); err != nil {
		t.Fatal(err)
	}
	if name != "Tourenrad Größe L" || status != "in_stock" {
		t.Errorf("imported article: %q %s", name, status)
	}

	// Preview: columns, first rows, detected separator and encoding.
	rec = do(t, h, "POST", "/api/csv-preview", "application/octet-stream", file)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"delimiter":";"`) || !strings.Contains(body, `"encoding":"windows-1252"`) ||
		!strings.Contains(body, `"columns":["Artikelnummer","Bezeichnung","Marke","EK","Bestand","Antrieb"]`) || !strings.Contains(body, "Tourenrad Größe L") {
		t.Errorf("preview: %d %s", rec.Code, body)
	}
	if rec := do(t, h, "POST", "/api/csv-preview", jsonType, file); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("preview with the wrong content type: %d", rec.Code)
	}
}

// fakeJobs records what was queued.
type fakeJobs struct {
	fetched []string
	syncs   int
}

func (f *fakeJobs) EnqueueSync(context.Context) error {
	f.syncs++
	return nil
}

func (f *fakeJobs) EnqueueFetch(_ context.Context, sourceID string) error {
	f.fetched = append(f.fetched, sourceID)
	return nil
}

func TestSourceFetchSettings(t *testing.T) {
	pool := dbtest.New(t)
	jobs := &fakeJobs{}
	h := New(pool, "tester", nil, WithJobs(jobs))
	const jsonType = "application/json"
	body := func(fetch string) string {
		return `{"name":"Hartje","connector_config":{"delimiter":";","columns":{"sku":"Nr"}},"fetch":` + fetch + `}`
	}
	cases := []struct {
		name, fetch string
		want        int
	}{
		{"no fetch settings", `{}`, http.StatusNoContent},
		{"not a web address", `{"url":"file:///etc/passwd"}`, http.StatusUnprocessableEntity},
		{"secret variable outside the prefix", `{"url":"https://feed.test/a.csv","auth":"bearer","secret_env":"DATABASE_URL"}`, http.StatusUnprocessableEntity},
		{"interval too short", `{"url":"https://feed.test/a.csv","interval_minutes":1}`, http.StatusUnprocessableEntity},
		{"valid", `{"url":" https://feed.test/a.csv ","interval_minutes":120,"auth":"basic","user_env":"VELOPIM_FETCH_HARTJE_USER","secret_env":"VELOPIM_FETCH_HARTJE_PASSWORD"}`, http.StatusNoContent},
	}
	// Without an address there is nothing to fetch.
	if rec := do(t, h, "PUT", "/api/sources/hartje", jsonType, body(`{}`)); rec.Code != http.StatusNoContent {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/api/sources/hartje/fetch", jsonType, `{}`); rec.Code != http.StatusConflict {
		t.Errorf("fetch without address: %d, want 409", rec.Code)
	}
	for _, c := range cases {
		if rec := do(t, h, "PUT", "/api/sources/hartje", jsonType, body(c.fetch)); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
	rec := do(t, h, "GET", "/api/sources", "", "")
	out := rec.Body.String()
	if !strings.Contains(out, `"url":"https://feed.test/a.csv"`) || !strings.Contains(out, `"interval_minutes":120`) ||
		!strings.Contains(out, `"user_env":"VELOPIM_FETCH_HARTJE_USER"`) || !strings.Contains(out, `"last_fetch_at":null`) {
		t.Errorf("stored fetch settings: %s", out)
	}

	// "Fetch now" queues a job; a form post or an unknown source does not.
	if rec := do(t, h, "POST", "/api/sources/hartje/fetch", "application/x-www-form-urlencoded", ""); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form post: %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/api/sources/nobody/fetch", jsonType, `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown source: %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/api/sources/hartje/fetch", jsonType, `{}`); rec.Code != http.StatusAccepted {
		t.Errorf("fetch now: %d %s", rec.Code, rec.Body)
	}
	if strings.Join(jobs.fetched, ",") != "hartje" {
		t.Errorf("queued: %v", jobs.fetched)
	}
	// Without a queue the request is refused.
	if rec := do(t, New(pool, "tester", nil), "POST", "/api/sources/hartje/fetch", jsonType, `{}`); rec.Code != http.StatusConflict {
		t.Errorf("without queue: %d", rec.Code)
	}
}

func TestChangesQueueASync(t *testing.T) {
	pool := dbtest.New(t)
	jobs := &fakeJobs{}
	h := New(pool, "tester", nil, WithJobs(jobs), WithRawDir(t.TempDir()))
	const jsonType = "application/json"
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source (id, name, connector_kind, connector_config) VALUES
		  ('example', 'Beispiel', 'csvfile', '{"delimiter": ";", "decimal_comma": true, "columns": {"sku": "Nr", "brand": "Marke", "name": "Name", "cost": "EK"}}');
	`); err != nil {
		t.Fatal(err)
	}
	// Each of these can change what Shopify should show and must queue a
	// sync; the read and the note must not.
	steps := []struct {
		name, method, path, contentType, body string
		wantStatus                            int
		queues                                bool
	}{
		{"import", "POST", "/api/sources/example/imports", "application/octet-stream", "Nr;Marke;Name;EK\n1;Beispielrad;City 3;500,00\n", http.StatusOK, true},
		{"read tasks", "GET", "/api/tasks", "", "", http.StatusOK, false},
		{"approve the model", "POST", "/api/tasks/1/decision", jsonType, `{"approve":true,"variant_prices":{"1":89900}}`, http.StatusNoContent, true},
		{"refused decision", "POST", "/api/tasks/1/decision", jsonType, `{"approve":true}`, http.StatusConflict, false},
		{"edit the product", "PATCH", "/api/products/1", jsonType, `{"name":"City 3 Neu"}`, http.StatusNoContent, true},
		{"refused edit", "PATCH", "/api/products/1", jsonType, `{"name":""}`, http.StatusUnprocessableEntity, false},
		{"set a price", "PATCH", "/api/variants/1", jsonType, `{"price_cents":84900}`, http.StatusNoContent, true},
		{"select for the store", "PATCH", "/api/products/1", jsonType, `{"wanted":true}`, http.StatusNoContent, true},
		{"note", "POST", "/api/products/1/notes", jsonType, `{"body":"intern"}`, http.StatusNoContent, false},
	}
	for _, s := range steps {
		before := jobs.syncs
		rec := do(t, h, s.method, s.path, s.contentType, s.body)
		if rec.Code != s.wantStatus {
			t.Fatalf("%s: status %d, want %d (%s)", s.name, rec.Code, s.wantStatus, rec.Body)
		}
		if queued := jobs.syncs > before; queued != s.queues {
			t.Errorf("%s: sync queued = %v, want %v", s.name, queued, s.queues)
		}
	}
}
