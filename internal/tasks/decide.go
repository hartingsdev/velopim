// Package tasks holds the task list. Closing an approval task decides the
// action behind it.
package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/db"
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
	q := db.New(tx)
	actionID, err := q.CloseOpenTask(ctx, db.CloseOpenTaskParams{ID: taskID, Status: taskStatus, ClosedBy: &actor, Note: &note})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotOpen
	}
	if err != nil {
		return err
	}
	if actionID != nil {
		n, err := q.DecidePendingAction(ctx, db.DecidePendingActionParams{
			ID: *actionID, Status: actionStatus, DecidedBy: &actor, DecisionNote: &note})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("tasks: action %d is not pending", *actionID)
		}
	}
	return tx.Commit(ctx)
}
