package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/dbtest"
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
		INSERT INTO action (type, status, supplier_item_id, field, old_value, new_value, rule_id, decision_reason)
		  VALUES ('price_change', 'pending_approval', 1, 'cost_cents', '100000', '95000', 1, 'most specific rule matched');
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
	h := New(pool, "tester")
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
		a.Rule == nil || *a.Rule != "prices need approval" {
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
	list := getTasks(t, New(pool, "tester"), "open")
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
	h := New(pool, "tester")

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
	h := New(pool, "tester")
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
	want := "City 3 Herren draft VP-100001 200-002 rot/50, City 3 Wave draft VP-100002 200-001 rot/50"
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
