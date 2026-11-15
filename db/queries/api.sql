-- name: ListOpenTasks :many
SELECT t.id, t.kind, t.title, t.detail, t.status, t.priority, t.created_at, t.closed_at, t.closed_by, t.note,
       t.import_run_id, t.action_id,
       a.type AS action_type, a.field AS action_field, a.old_value, a.new_value, a.decision_reason,
       r.name AS rule_name,
       s.source_id, s.sku, s.brand, s.name AS item_name, s.model_year, s.color, s.frame_size
FROM task t
LEFT JOIN action a ON a.id = t.action_id
LEFT JOIN rule r ON r.id = a.rule_id
LEFT JOIN supplier_item s ON s.id = t.supplier_item_id
WHERE t.status = 'open'
ORDER BY t.priority, t.created_at DESC, t.id DESC
LIMIT $1;

-- name: ListClosedTasks :many
SELECT t.id, t.kind, t.title, t.detail, t.status, t.priority, t.created_at, t.closed_at, t.closed_by, t.note,
       t.import_run_id, t.action_id,
       a.type AS action_type, a.field AS action_field, a.old_value, a.new_value, a.decision_reason,
       r.name AS rule_name,
       s.source_id, s.sku, s.brand, s.name AS item_name, s.model_year, s.color, s.frame_size
FROM task t
LEFT JOIN action a ON a.id = t.action_id
LEFT JOIN rule r ON r.id = a.rule_id
LEFT JOIN supplier_item s ON s.id = t.supplier_item_id
WHERE t.status <> 'open'
ORDER BY t.closed_at DESC, t.id DESC
LIMIT $1;

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
SELECT ai.action_id, s.source_id, s.sku, s.brand, s.name, s.model_year, s.color, s.frame_size
FROM action_item ai JOIN supplier_item s ON s.id = ai.supplier_item_id
WHERE ai.action_id = ANY(sqlc.arg(action_ids)::bigint[])
ORDER BY ai.action_id, s.color, s.frame_size, s.sku;
