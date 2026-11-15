-- name: GetAvailabilityMapping :one
SELECT availability_mapping FROM source WHERE id = $1;

-- name: ListEnabledRules :many
SELECT id, name, enabled, action_type, scope_source_id, scope_brand, scope_category, conditions, outcome, guard
FROM rule WHERE enabled;

-- name: ListPricingRules :many
SELECT scope_brand, scope_category, config FROM pricing_rule;

-- name: ListSupplierItems :many
SELECT s.id, s.active, s.raw_record_id, coalesce(r.content_hash, '')::text AS raw_hash,
       s.sku, s.ean, s.mpn, s.brand, s.name, s.model_year, s.category, s.color, s.frame_size,
       s.description, s.image_urls, s.cost_cents, s.rrp_cents, s.attributes, s.availability
FROM supplier_item s LEFT JOIN raw_record r ON r.id = s.raw_record_id
WHERE s.source_id = $1;

-- name: CreateImportRun :one
INSERT INTO import_run (source_id, status, file_path, file_sha256, record_count, started_at)
VALUES ($1, 'running', $2, $3, $4, $5) RETURNING id;

-- name: FinishImportRun :exec
UPDATE import_run SET status = $2, issue_count = $3, summary = $4, finished_at = $5 WHERE id = $1;

-- name: InsertRawRecord :one
INSERT INTO raw_record (source_id, key, content_hash, fields, first_import_run_id)
VALUES ($1, $2, $3, $4, $5) RETURNING id;

-- name: InsertSupplierItem :one
INSERT INTO supplier_item (source_id, sku, ean, mpn, brand, name, model_year, category, color, frame_size,
    description, image_urls, cost_cents, rrp_cents, attributes, availability, availability_status,
    raw_record_id, last_import_run_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
RETURNING id;

-- name: UpdateSupplierItem :exec
UPDATE supplier_item SET ean = $3, mpn = $4, brand = $5, name = $6, model_year = $7, category = $8, color = $9,
    frame_size = $10, description = $11, image_urls = $12, cost_cents = $13, rrp_cents = $14, attributes = $15,
    availability = $16, availability_status = $17, raw_record_id = $18, last_import_run_id = $19,
    active = true, updated_at = now()
WHERE source_id = $1 AND sku = $2;

-- name: DeactivateSupplierItem :exec
UPDATE supplier_item SET active = false, updated_at = now() WHERE id = $1;

-- name: InsertImportFieldChange :exec
INSERT INTO field_change (entity_type, entity_id, field, old_value, new_value, origin, import_run_id)
VALUES ('supplier_item', $1, $2, $3, $4, 'import', $5);

-- name: SupersedePendingActions :many
UPDATE action SET status = 'superseded', decided_at = now(), decided_by = 'system'
WHERE supplier_item_id = $1 AND type = $2 AND field = $3 AND status = 'pending_approval'
RETURNING id, old_value, base_price_cents, proposed_price_cents;

-- name: DismissTasksOfActions :exec
UPDATE task SET status = 'dismissed', closed_at = now(), closed_by = 'system', note = 'Durch neuere Änderung ersetzt'
WHERE action_id = ANY(sqlc.arg(action_ids)::bigint[]) AND status = 'open';

-- name: InsertAction :one
INSERT INTO action (type, status, supplier_item_id, field, old_value, new_value, facts, rule_id,
    decision_reason, import_run_id, decided_at, decided_by, model_key, variant_id, base_price_cents,
    proposed_price_cents)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id;

-- name: InsertTask :exec
INSERT INTO task (kind, title, detail, priority, action_id, supplier_item_id, import_run_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: SupersedePendingModelActions :many
UPDATE action SET status = 'superseded', decided_at = now(), decided_by = 'system'
WHERE type = 'new_product' AND model_key = $1 AND status = 'pending_approval'
RETURNING id;

-- name: ListActionItemIDs :many
SELECT DISTINCT supplier_item_id FROM action_item
WHERE action_id = ANY(sqlc.arg(action_ids)::bigint[])
ORDER BY supplier_item_id;

-- name: InsertActionItems :exec
INSERT INTO action_item (action_id, supplier_item_id)
SELECT sqlc.arg(action_id)::bigint, unnest(sqlc.arg(item_ids)::bigint[]);

-- name: ModelWasAccepted :one
SELECT EXISTS (
    SELECT 1 FROM action
    WHERE type = 'new_product' AND model_key = $1 AND status IN ('approved', 'auto_applied')
);
