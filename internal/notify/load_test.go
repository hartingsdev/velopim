package notify

import (
	"context"
	"testing"

	"velopim/internal/dbtest"
)

func TestLoadCountsOnlyWhatIsDueNow(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source (id, name, connector_kind) VALUES ('example', 'Beispiel', 'csvfile');
		INSERT INTO supplier_item (source_id, sku) VALUES ('example', '1');
		INSERT INTO action (type, status, supplier_item_id, field, decision_reason) VALUES
		  ('price_change', 'pending_approval', 1, 'cost_cents', 'x'),
		  ('price_change', 'pending_approval', 1, 'rrp_cents', 'x'),
		  ('price_change', 'pending_approval', 1, 'x', 'x');
		INSERT INTO task (kind, title, priority, action_id, status, snoozed_until) VALUES
		  ('approve_action', 'offen, hoch', 1, 1, 'open', NULL),
		  ('approve_action', 'aus der Wiedervorlage zurück', 2, 2, 'open', now() - interval '1 hour'),
		  ('approve_action', 'noch auf Wiedervorlage', 1, 3, 'open', now() + interval '2 days'),
		  ('data_issue', 'erledigt', 3, NULL, 'done', NULL),
		  ('data_issue', 'Datenprobleme', 3, NULL, 'open', NULL);
	`); err != nil {
		t.Fatal(err)
	}
	c, err := Load(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	// The task put aside until the day after tomorrow and the closed one do
	// not count.
	if c.Open != 3 || c.High != 1 || c.Resubmitted != 1 {
		t.Errorf("counts: %+v", c)
	}
	if len(c.ByCategory) != 2 || c.ByCategory[0] != (CategoryCount{"Preise", 2}) || c.ByCategory[1] != (CategoryCount{"Import", 1}) {
		t.Errorf("by category: %+v", c.ByCategory)
	}
}
