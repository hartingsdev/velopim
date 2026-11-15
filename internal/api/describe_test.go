package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"velopim/internal/dbtest"
	"velopim/internal/describe"
)

// fakeDescriber records what it was asked.
type fakeDescriber struct {
	asked []describe.Input
	err   error
}

func (f *fakeDescriber) Generate(_ context.Context, in describe.Input) (describe.Draft, error) {
	f.asked = append(f.asked, in)
	if f.err != nil {
		return describe.Draft{}, f.err
	}
	return describe.Draft{Paragraphs: []string{"Ein Rad für die Stadt."}, Highlights: []string{"Nexus 7"}}, nil
}

func TestDescriptionDraft(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	const jsonType = "application/json"
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Geheimer Lieferant', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, model_year, category, color, frame_size, cost_cents, rrp_cents, description, attributes) VALUES
		  ('example', '200-001', 'Beispielrad', 'City 3', 2027, 'Cityrad', 'rot', '50', 51234, 89900, 'Lieferantentext zum City 3.', '{"Schaltung": "Nexus 7", "Bremse": "V-Brake"}'),
		  ('example', '200-002', 'Beispielrad', 'City 3', 2027, 'Cityrad', 'rot', '54', 51234, 89900, 'Lieferantentext zum City 3.', '{"Schaltung": "Nexus 7", "Bremse": "Rücktritt"}');
		INSERT INTO action (type, status, new_value, decision_reason, model_key)
		  VALUES ('new_product', 'pending_approval', 'Beispielrad City 3 (2027)', 'no rule matched, approval by default', 'm');
		INSERT INTO action_item (action_id, supplier_item_id) SELECT 1, id FROM supplier_item;
		INSERT INTO task (kind, title, priority, action_id) VALUES ('approve_action', 'Neues Modell prüfen', 2, 1);
	`); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDescriber{}
	h := New(pool, "tester", nil, WithDescriber(fake))
	if rec := do(t, h, "POST", "/api/tasks/1/decision", jsonType,
		`{"approve":true,"variant_prices":{"200-001":89900,"200-002":89900}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/api/products/1/notes", jsonType, `{"body":"Interne Notiz: Marge knapp"}`); rec.Code >= 300 {
		t.Fatalf("note: %d %s", rec.Code, rec.Body)
	}
	var before string
	if err := pool.QueryRow(ctx, `SELECT description_html FROM product WHERE id = 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	draft := func(wantCode int) string {
		t.Helper()
		rec := do(t, h, "POST", "/api/products/1/description-draft", jsonType, `{}`)
		if rec.Code != wantCode {
			t.Fatalf("draft: %d %s, want %d", rec.Code, rec.Body, wantCode)
		}
		var out struct {
			Description string `json:"description_html"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Description
	}

	// The draft comes back as HTML and is not stored.
	if got := draft(http.StatusOK); got != "<p>Ein Rad für die Stadt.</p>\n<ul>\n  <li>Nexus 7</li>\n</ul>" {
		t.Errorf("draft = %q", got)
	}
	var after string
	if err := pool.QueryRow(ctx, `SELECT description_html FROM product WHERE id = 1`).Scan(&after); err != nil || after != before {
		t.Errorf("the draft changed the product: %q -> %q (%v)", before, after, err)
	}

	// What the model was told: product, choices, technical data, supplier
	// text. No purchase price, supplier name, article number or note.
	in := fake.asked[0]
	want := describe.Input{Brand: "Beispielrad", Name: in.Name, ModelYear: 2027, Category: "Cityrad",
		Options:      in.Options,
		Attributes:   map[string][]string{"Schaltung": {"Nexus 7"}, "Bremse": {"V-Brake", "Rücktritt"}},
		SupplierText: "Lieferantentext zum City 3."}
	if !reflect.DeepEqual(in, want) || !strings.Contains(in.Name, "City 3") {
		t.Errorf("asked with %+v", in)
	}
	sizes := false
	for _, o := range in.Options {
		sizes = sizes || reflect.DeepEqual(o.Values, []string{"50", "54"})
	}
	if !sizes {
		t.Errorf("options = %+v", in.Options)
	}
	prompt := describe.Prompt(in)
	for _, secret := range []string{"512", "Geheimer Lieferant", "200-001", "Marge", "899"} {
		if strings.Contains(prompt, secret) {
			t.Errorf("prompt contains %q:\n%s", secret, prompt)
		}
	}

	// Usage policy: texts that may not be used in the shop are not material
	// either; ignored technical data is left out.
	if _, err := pool.Exec(ctx, `INSERT INTO usage_policy (source_id, field_group, policy) VALUES
		('example', 'texts', 'internal_only'), ('example', 'attributes', 'ignore')`); err != nil {
		t.Fatal(err)
	}
	draft(http.StatusOK)
	if in := fake.asked[1]; in.SupplierText != "" || len(in.Attributes) != 0 {
		t.Errorf("policy not applied: %+v", in)
	}
	if _, err := pool.Exec(ctx, `UPDATE usage_policy SET policy = 'internal_only' WHERE field_group = 'attributes'`); err != nil {
		t.Fatal(err)
	}
	draft(http.StatusOK)
	if in := fake.asked[2]; in.SupplierText != "" || len(in.Attributes) != 2 {
		t.Errorf("internal-only technical data should be material: %+v", in)
	}

	// Failures and guards.
	fake.err = errors.New("boom")
	draft(http.StatusBadGateway)
	fake.err = nil
	if rec := do(t, h, "POST", "/api/products/99/description-draft", jsonType, `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown product: %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/api/products/1/description-draft", "text/plain", `{}`); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form post: %d", rec.Code)
	}

	// Without a text model the product says so and the endpoint refuses.
	plain := New(pool, "tester", nil)
	if rec := do(t, plain, "POST", "/api/products/1/description-draft", jsonType, `{}`); rec.Code != http.StatusConflict {
		t.Errorf("not configured: %d", rec.Code)
	}
	for handler, want := range map[http.Handler]bool{h: true, plain: false} {
		var d ProductDetail
		_ = json.Unmarshal(do(t, handler, "GET", "/api/products/1", "", "").Body.Bytes(), &d)
		if d.CanDraftText != want {
			t.Errorf("can_draft_text = %v, want %v", d.CanDraftText, want)
		}
	}
}
