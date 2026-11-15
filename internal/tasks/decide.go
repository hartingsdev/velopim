// Package tasks holds the task list. Closing an approval task decides the
// action behind it.
package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/catalog"
	"velopim/internal/db"
)

// ErrNotOpen is returned when a task is already closed or does not exist.
var ErrNotOpen = errors.New("tasks: task is not open")

// Decision is what a person decides about a task.
type Decision struct {
	Approve bool
	// Actor is who decides; required.
	Actor string
	Note  string

	// The remaining fields correct the proposal before it is approved and
	// are ignored when rejecting.

	// SplitBy says which axes separate products when a new model is
	// approved (docs/decisions/0021).
	SplitBy []catalog.Axis
	// VariantPrices sets the sales price per supplier article number when a
	// new model is approved, instead of the calculated price.
	VariantPrices map[string]int64
	// PriceCents sets the sales price when a purchase price change is
	// approved, instead of the proposed price.
	PriceCents *int64
}

// Decide closes an open task. For an approval task the action behind it
// becomes approved or rejected; who decided, when and why is recorded on
// both.
//
// Approving a new-model action creates its products and variants. Approving
// a purchase price change with a price proposal sets the variant's sales
// price. A *catalog.BlockedError means the approval is not possible as asked
// and nothing was changed.
func Decide(ctx context.Context, pool *pgxpool.Pool, taskID int64, d Decision) error {
	if d.Actor == "" {
		return errors.New("tasks: actor is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	taskStatus, actionStatus := "done", "approved"
	if !d.Approve {
		taskStatus, actionStatus = "dismissed", "rejected"
	}
	q := db.New(tx)
	actionID, err := q.CloseOpenTask(ctx, db.CloseOpenTaskParams{ID: taskID, Status: taskStatus, ClosedBy: &d.Actor, Note: &d.Note})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotOpen
	}
	if err != nil {
		return err
	}
	if actionID != nil {
		action, err := q.DecidePendingAction(ctx, db.DecidePendingActionParams{
			ID: *actionID, Status: actionStatus, DecidedBy: &d.Actor, DecisionNote: &d.Note})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("tasks: action %d is not pending", *actionID)
		}
		if err != nil {
			return err
		}
		if d.Approve {
			if err := apply(ctx, q, *actionID, action, d); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// apply carries out an approved action in the own catalog.
func apply(ctx context.Context, q *db.Queries, actionID int64, action db.DecidePendingActionRow, d Decision) error {
	switch {
	case action.Type == "new_product" && action.ModelKey != "":
		return catalog.CreateFromAction(ctx, q, actionID, action.ModelKey, d.SplitBy, d.VariantPrices, d.Actor)

	case action.Type == "price_change":
		hasProposal := action.VariantID != nil && action.BasePriceCents != nil && action.ProposedPriceCents != nil
		if d.PriceCents != nil && (!hasProposal || *d.PriceCents != *action.ProposedPriceCents) {
			if action.VariantID == nil {
				return &catalog.BlockedError{Issues: []string{"Zu diesem Artikel gibt es keine Variante, deren Preis sich setzen lässt"}}
			}
			if err := q.SetActionProposedPrice(ctx, db.SetActionProposedPriceParams{ID: actionID, ProposedPriceCents: d.PriceCents}); err != nil {
				return err
			}
			return catalog.SetVariantPrice(ctx, q, *action.VariantID, *d.PriceCents, actionID, d.Actor,
				"Preis bei der Freigabe korrigiert")
		}
		if hasProposal {
			delta := *action.ProposedPriceCents - *action.BasePriceCents
			return catalog.ShiftVariantPrice(ctx, q, *action.VariantID, delta, "user", actionID, d.Actor,
				"Preisänderung freigegeben")
		}
	}
	return nil
}
