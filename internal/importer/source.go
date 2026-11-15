package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/connector"
	"velopim/internal/connector/csvfile"
	"velopim/internal/db"
)

// ConnectorFor builds the connector a source is configured with.
func ConnectorFor(ctx context.Context, q *db.Queries, sourceID string) (connector.Connector, error) {
	src, err := q.GetSource(ctx, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("importer: source %q is not configured", sourceID)
	}
	if err != nil {
		return nil, err
	}
	switch src.ConnectorKind {
	case "csvfile":
		var cfg csvfile.Config
		if err := json.Unmarshal(src.ConnectorConfig, &cfg); err != nil {
			return nil, fmt.Errorf("importer: connector config of %q: %w", sourceID, err)
		}
		cfg.SourceID = src.ID
		return csvfile.New(cfg)
	}
	return nil, fmt.Errorf("importer: source %q has unknown connector kind %q", sourceID, src.ConnectorKind)
}

// Errors of ConfirmHeld.
var (
	ErrRunNotHeld   = errors.New("importer: import run is not held")
	ErrRunNotLatest = errors.New("importer: a newer import exists for this source")
)

// ConfirmHeld accepts a held delivery as complete: the stored original file
// is imported again without the drop check, so the missing articles are
// deactivated, and the task about the held run is closed.
//
// It is only allowed for the latest run of the source; importing an older
// file again would turn the supplier view back.
func ConfirmHeld(ctx context.Context, pool *pgxpool.Pool, runID int64, actor string, opt Options) (Result, error) {
	q := db.New(pool)
	run, err := q.GetImportRun(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrRunNotHeld
	}
	if err != nil {
		return Result{}, err
	}
	if run.Status != "held" || run.FilePath == nil {
		return Result{}, ErrRunNotHeld
	}
	latest, err := q.LatestImportRunID(ctx, run.SourceID)
	if err != nil {
		return Result{}, err
	}
	if latest != runID {
		return Result{}, ErrRunNotLatest
	}
	conn, err := ConnectorFor(ctx, q, run.SourceID)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(*run.FilePath)
	if err != nil {
		return Result{}, fmt.Errorf("importer: stored delivery of run %d: %w", runID, err)
	}
	defer f.Close()

	opt.MaxDropPct, opt.confirming = 100, true
	res, err := Run(ctx, pool, conn, f, opt)
	if err != nil {
		return Result{}, err
	}
	return res, q.CloseImportHeldTasks(ctx, db.CloseImportHeldTasksParams{ImportRunID: &runID, ClosedBy: &actor})
}
