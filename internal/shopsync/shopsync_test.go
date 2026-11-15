package shopsync_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/dbtest"
	"velopim/internal/shopify"
	"velopim/internal/shopsync"
	"velopim/internal/tasks"
)

// fakeStore records what would be sent to Shopify.
type fakeStore struct {
	calls  []shopify.Product
	ids    []string // Shopify id each call was made with
	fail   error
	nextID int
}

func (f *fakeStore) SetProduct(_ context.Context, p shopify.Product, shopifyID string) (shopify.SetResult, error) {
	f.calls = append(f.calls, p)
	f.ids = append(f.ids, shopifyID)
	if f.fail != nil {
		return shopify.SetResult{}, f.fail
	}
	if shopifyID == "" {
		f.nextID++
		shopifyID = "gid://shopify/Product/" + strconv.Itoa(f.nextID)
	}
	res := shopify.SetResult{ProductID: shopifyID, NumericID: strings.TrimPrefix(shopifyID, "gid://shopify/Product/"),
		VariantIDs: map[string]string{}}
	for _, v := range p.Variants {
		res.VariantIDs[v.SKU] = "gid://shopify/ProductVariant/" + v.SKU
	}
	return res, nil
}

func (f *fakeStore) AdminURL(id string) string { return "https://admin.example/products/" + id }

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

func TestSync(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku, brand, name, cost_cents, availability, availability_status) VALUES
		  ('example', '200-001', 'Beispielrad', 'City 3', 50000, '{"status": "in_stock"}', 'in_stock'),
		  ('example', '200-002', 'Beispielrad', 'City 3', 50000, '{"status": "unavailable"}', 'unavailable');
		INSERT INTO product (brand, name, option_names) VALUES ('Beispielrad', 'City 3', '["Rahmenhöhe"]'), ('Beispielrad', 'Nicht gewählt', '[]');
		INSERT INTO variant (product_id, sku, option_values, price_cents) VALUES (1, '10001', '["50"]', 89900), (1, '10002', '["54"]', 89900);
		INSERT INTO variant_supplier_item (variant_id, supplier_item_id, preferred) VALUES (1, 1, true), (2, 2, true);
		INSERT INTO shopify_listing (product_id, wanted) VALUES (1, true), (2, false);
	`); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{}
	run := func() shopsync.Result {
		t.Helper()
		res, err := shopsync.Run(ctx, pool, store, now)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// First run: the selected product is created as a draft; the unselected
	// one is left alone. A task asks to check and publish it.
	res := run()
	if res.Synced != 1 || res.Failed != 0 || len(store.calls) != 1 || store.ids[0] != "" {
		t.Fatalf("first run: %+v, %d calls", res, len(store.calls))
	}
	sent := store.calls[0]
	if sent.Published || sent.Title != "City 3" || len(sent.Variants) != 2 || !sent.Variants[0].Sellable || sent.Variants[1].Sellable {
		t.Errorf("sent product: %+v", sent)
	}
	if got := one(t, pool, `SELECT shopify_product_id || ' ' || (variant_ids->>'10001') || ' ' || (synced_projection IS NOT NULL) || ' ' || last_synced_at::date
		FROM shopify_listing WHERE product_id = 1`); got != "gid://shopify/Product/1 gid://shopify/ProductVariant/10001 true 2027-01-10" {
		t.Errorf("listing: %s", got)
	}
	if got := one(t, pool, `SELECT title || ' | ' || detail FROM task WHERE kind = 'publish_product' AND status = 'open'`); !strings.HasPrefix(got, "Veröffentlichen: Beispielrad City 3 | ") ||
		!strings.Contains(got, "https://admin.example/products/1") {
		t.Errorf("publish task: %s", got)
	}

	// Nothing changed: nothing is sent.
	if res := run(); res.Unchanged != 1 || res.Synced != 0 || len(store.calls) != 1 {
		t.Errorf("second run must send nothing: %+v, %d calls", res, len(store.calls))
	}

	// Approving the publish task makes the product active; the next run
	// publishes the existing Shopify product and creates no second task.
	var taskID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM task WHERE kind = 'publish_product'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Decide(ctx, pool, taskID, tasks.Decision{Approve: true, Actor: "robert"}); err != nil {
		t.Fatal(err)
	}
	if got := one(t, pool, `SELECT p.status || ' ' || fc.old_value || '>' || fc.new_value || ' ' || fc.actor FROM product p
		JOIN field_change fc ON fc.entity_type = 'product' AND fc.entity_id = p.id AND fc.field = 'status' WHERE p.id = 1`); got != "active draft>active robert" {
		t.Errorf("after publishing: %s", got)
	}
	run()
	if len(store.calls) != 2 || !store.calls[1].Published || store.ids[1] != "gid://shopify/Product/1" {
		t.Fatalf("publishing run: %d calls, ids %v", len(store.calls), store.ids)
	}
	if n := one(t, pool, `SELECT count(*)::text FROM task WHERE kind = 'publish_product'`); n != "1" {
		t.Errorf("publish tasks = %s, want 1", n)
	}

	// The supplier sells out: the variant is no longer sellable. Shopify
	// refuses twice: the error is stored and there is one task, not two.
	if _, err := pool.Exec(ctx, `UPDATE supplier_item SET availability = '{"status": "unavailable"}' WHERE sku = '200-001'`); err != nil {
		t.Fatal(err)
	}
	store.fail = &shopify.UserError{Messages: []string{"variants.0.price: is invalid"}}
	for range 2 {
		if res := run(); res.Failed != 1 || res.Products[0].Error == "" {
			t.Fatalf("failing run: %+v", res)
		}
	}
	if got := one(t, pool, `SELECT count(*) || ' ' || max(priority) || ' ' || max(detail) FROM task WHERE kind = 'sync_failed' AND status = 'open'`); got != "1 1 shopify: variants.0.price: is invalid" {
		t.Errorf("failure task: %s", got)
	}
	if got := one(t, pool, `SELECT last_error FROM shopify_listing WHERE product_id = 1`); got != "shopify: variants.0.price: is invalid" {
		t.Errorf("stored error: %s", got)
	}

	// It works again: the change arrives, the error and its task are gone.
	store.fail = nil
	run()
	last := store.calls[len(store.calls)-1]
	if last.Variants[0].Sellable {
		t.Error("sold-out variant is still sellable")
	}
	if got := one(t, pool, `SELECT coalesce(l.last_error, 'ok') || ' ' || (SELECT count(*) FROM task WHERE kind = 'sync_failed' AND status = 'open')
		FROM shopify_listing l WHERE l.product_id = 1`); got != "ok 0" {
		t.Errorf("after recovery: %s", got)
	}

	// Taken out of the store: the Shopify product goes back to draft.
	if _, err := pool.Exec(ctx, `UPDATE shopify_listing SET wanted = false WHERE product_id = 1`); err != nil {
		t.Fatal(err)
	}
	before := len(store.calls)
	run()
	if len(store.calls) != before+1 || store.calls[before].Published {
		t.Errorf("unselected product must be sent as draft: %d new calls", len(store.calls)-before)
	}
	if res := run(); res.Unchanged != 1 {
		t.Errorf("after unselecting, a further run must send nothing: %+v", res)
	}
}

func TestSyncStopsWhenWritingIsNotAllowed(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO product (brand, name) VALUES ('Beispielrad', 'City 3');
		INSERT INTO variant (product_id, sku, price_cents) VALUES (1, '10001', 89900);
		INSERT INTO shopify_listing (product_id, wanted) VALUES (1, true);
	`); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{fail: shopify.ErrWritesDisabled}
	if _, err := shopsync.Run(ctx, pool, store, time.Now()); !errors.Is(err, shopify.ErrWritesDisabled) {
		t.Fatalf("got %v, want ErrWritesDisabled", err)
	}
	// A switched-off sync is no fault of the product: no error, no task.
	if got := one(t, pool, `SELECT coalesce(l.last_error, 'none') || ' ' || (SELECT count(*) FROM task) FROM shopify_listing l`); got != "none 0" {
		t.Errorf("after refused run: %s", got)
	}
}
