-- name: ListRules :many
SELECT id, name, enabled, action_type, scope_source_id, scope_brand, scope_category, conditions, outcome, guard
FROM rule ORDER BY action_type, scope_source_id, scope_brand, scope_category, id;

-- name: InsertRule :one
INSERT INTO rule (name, enabled, action_type, scope_source_id, scope_brand, scope_category, conditions, outcome, guard)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: UpdateRule :execrows
UPDATE rule SET name = $2, enabled = $3, action_type = $4, scope_source_id = $5, scope_brand = $6,
    scope_category = $7, conditions = $8, outcome = $9, guard = $10, updated_at = now()
WHERE id = $1;

-- name: RuleIsUsed :one
SELECT (EXISTS (SELECT 1 FROM action a WHERE a.rule_id = sqlc.arg(id)::bigint)
    OR EXISTS (SELECT 1 FROM field_change fc WHERE fc.rule_id = sqlc.arg(id)::bigint))::bool AS used;

-- name: DeleteRule :execrows
DELETE FROM rule WHERE id = $1;

-- name: ListSources :many
SELECT id, name FROM source ORDER BY name;
