-- name: CloseOpenTask :one
UPDATE task SET status = $2, closed_at = now(), closed_by = $3, note = $4
WHERE id = $1 AND status = 'open'
RETURNING action_id;

-- name: DecidePendingAction :execrows
UPDATE action SET status = $2, decided_at = now(), decided_by = $3, decision_note = $4
WHERE id = $1 AND status = 'pending_approval';
