// Package tasks holds the task list. Closing an approval task decides the
// action behind it.
package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotOpen is returned when a task is already closed or does not exist.
var ErrNotOpen = errors.New("tasks: task is not open")

// Decide closes an open task. For an approval task the action behind it
// becomes approved or rejected; who decided, when and why is recorded on
// both. actor must not be empty.
func Decide(ctx context.Context, pool *pgxpool.Pool, taskID int64, approve bool, actor, note string) error {
	if actor == "" {
		return errors.New("tasks: actor is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	taskStatus, actionStatus := "done", "approved"
	if !approve {
		taskStatus, actionStatus = "dismissed", "rejected"
	}
	var actionID *int64
	err = tx.QueryRow(ctx,
		`UPDATE task SET status = $2, closed_at = now(), closed_by = $3, note = $4
		 WHERE id = $1 AND status = 'open' RETURNING action_id`,
		taskID, taskStatus, actor, note).Scan(&actionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotOpen
	}
	if err != nil {
		return err
	}
	if actionID != nil {
		tag, err := tx.Exec(ctx,
			`UPDATE action SET status = $2, decided_at = now(), decided_by = $3, decision_note = $4
			 WHERE id = $1 AND status = 'pending_approval'`,
			*actionID, actionStatus, actor, note)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("tasks: action %d is not pending", *actionID)
		}
	}
	return tx.Commit(ctx)
}
