-- name: ListTasks :many
-- Open tasks are ordered by priority, closed ones by when they were closed.
-- category '' means all; q matches title, article numbers and EANs.
SELECT t.id, t.kind, t.title, t.detail, t.status, t.priority, t.created_at, t.closed_at, t.closed_by, t.note,
       t.import_run_id, t.action_id, t.product_id,
       a.type AS action_type, a.field AS action_field, a.old_value, a.new_value, a.decision_reason, a.model_key, a.base_price_cents, a.proposed_price_cents,
       r.name AS rule_name,
       s.source_id, s.sku, s.brand, s.name AS item_name, s.model_year, s.color, s.frame_size,
       (CASE
           WHEN a.type = 'new_product' THEN 'new_model'
           WHEN a.type = 'price_change' THEN 'price'
           WHEN a.type = 'availability_status_change' THEN 'availability'
           WHEN a.type = 'deactivation' THEN 'removed'
           WHEN a.type IS NOT NULL THEN 'content'
           WHEN t.kind IN ('import_held', 'data_issue') THEN 'import'
           WHEN t.kind IN ('publish_product', 'sync_failed') THEN 'shopify'
           ELSE 'other'
       END)::text AS category
FROM task t
LEFT JOIN action a ON a.id = t.action_id
LEFT JOIN rule r ON r.id = a.rule_id
LEFT JOIN supplier_item s ON s.id = t.supplier_item_id
WHERE (t.status = 'open') = sqlc.arg(open)::bool
  AND (sqlc.arg(category)::text = '' OR (CASE
           WHEN a.type = 'new_product' THEN 'new_model'
           WHEN a.type = 'price_change' THEN 'price'
           WHEN a.type = 'availability_status_change' THEN 'availability'
           WHEN a.type = 'deactivation' THEN 'removed'
           WHEN a.type IS NOT NULL THEN 'content'
           WHEN t.kind IN ('import_held', 'data_issue') THEN 'import'
           WHEN t.kind IN ('publish_product', 'sync_failed') THEN 'shopify'
           ELSE 'other'
       END) = sqlc.arg(category)::text)
  AND (sqlc.arg(q)::text = ''
       OR strpos(lower(t.title), lower(sqlc.arg(q)::text)) > 0
       OR strpos(lower(coalesce(s.sku, '') || ' ' || coalesce(s.ean, '')), lower(sqlc.arg(q)::text)) > 0
       OR EXISTS (SELECT 1 FROM action_item ai JOIN supplier_item si ON si.id = ai.supplier_item_id
                  WHERE ai.action_id = t.action_id
                    AND strpos(lower(si.sku || ' ' || si.ean), lower(sqlc.arg(q)::text)) > 0))
ORDER BY CASE WHEN sqlc.arg(open)::bool THEN t.priority END,
         CASE WHEN sqlc.arg(open)::bool THEN t.created_at ELSE t.closed_at END DESC,
         t.id DESC
LIMIT sqlc.arg(max_rows);

-- name: CountTasksByCategory :many
SELECT (CASE
           WHEN a.type = 'new_product' THEN 'new_model'
           WHEN a.type = 'price_change' THEN 'price'
           WHEN a.type = 'availability_status_change' THEN 'availability'
           WHEN a.type = 'deactivation' THEN 'removed'
           WHEN a.type IS NOT NULL THEN 'content'
           WHEN t.kind IN ('import_held', 'data_issue') THEN 'import'
           WHEN t.kind IN ('publish_product', 'sync_failed') THEN 'shopify'
           ELSE 'other'
       END)::text AS category, count(*) AS count
FROM task t
LEFT JOIN action a ON a.id = t.action_id
LEFT JOIN supplier_item s ON s.id = t.supplier_item_id
WHERE (t.status = 'open') = sqlc.arg(open)::bool
  AND (sqlc.arg(q)::text = ''
       OR strpos(lower(t.title), lower(sqlc.arg(q)::text)) > 0
       OR strpos(lower(coalesce(s.sku, '') || ' ' || coalesce(s.ean, '')), lower(sqlc.arg(q)::text)) > 0
       OR EXISTS (SELECT 1 FROM action_item ai JOIN supplier_item si ON si.id = ai.supplier_item_id
                  WHERE ai.action_id = t.action_id
                    AND strpos(lower(si.sku || ' ' || si.ean), lower(sqlc.arg(q)::text)) > 0))
GROUP BY 1;

-- name: CountTasks :one
SELECT count(*) FILTER (WHERE status = 'open') AS open,
       count(*) FILTER (WHERE status <> 'open') AS closed
FROM task;

-- name: ListImportRuns :many
SELECT r.id, r.source_id, s.name AS source_name, r.status, r.record_count, r.issue_count, r.summary, r.error,
       r.started_at, r.finished_at
FROM import_run r JOIN source s ON s.id = r.source_id
ORDER BY r.started_at DESC, r.id DESC
LIMIT $1;

-- name: GetSource :one
SELECT id, name, connector_kind, connector_config FROM source WHERE id = $1;

-- name: ListActionItems :many
SELECT ai.action_id, s.source_id, s.sku, s.brand, s.name, s.model_year, s.category, s.color, s.frame_size,
       s.cost_cents, s.rrp_cents, s.attributes
FROM action_item ai JOIN supplier_item s ON s.id = ai.supplier_item_id
WHERE ai.action_id = ANY(sqlc.arg(action_ids)::bigint[])
ORDER BY ai.action_id, s.color, s.frame_size, s.sku;

-- name: SearchSupplierItems :many
SELECT s.id, s.source_id, s.sku, s.ean, s.brand, s.name, s.model_year, s.color, s.frame_size, s.cost_cents,
       s.rrp_cents, s.availability_status, s.active, v.sku AS own_sku, p.name AS product_name
FROM supplier_item s
LEFT JOIN variant_supplier_item vs ON vs.supplier_item_id = s.id
LEFT JOIN variant v ON v.id = vs.variant_id
LEFT JOIN product p ON p.id = v.product_id
WHERE strpos(lower(s.sku || ' ' || s.ean || ' ' || s.mpn || ' ' || s.brand || ' ' || s.name || ' ' || s.color),
             lower(sqlc.arg(q)::text)) > 0
ORDER BY s.brand, s.name, s.sku
LIMIT sqlc.arg(max_rows);
