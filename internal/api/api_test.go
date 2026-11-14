package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
