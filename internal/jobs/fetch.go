// Package jobs holds the background work: fetching supplier files on a
// schedule and syncing to Shopify. The queue is River, stored in PostgreSQL.
package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/db"
	"velopim/internal/fetch"
	"velopim/internal/importer"
)

// Fetcher fetches a source's file and imports it.
type Fetcher struct {
	Pool   *pgxpool.Pool
	RawDir string
	// HTTP may be nil for the default client.
	HTTP *http.Client
	// Getenv looks up secrets; nil means none are available.
	Getenv func(string) string
	Now    func() time.Time
}

func (f *Fetcher) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

// ErrNothingToFetch is returned for a source without a fetch address.
var ErrNothingToFetch = errors.New("jobs: the source has no fetch address")

// FetchSource downloads the file of a source and runs it through the import
// pipeline. The outcome is stored on the source. A failure becomes a task
// (one open per source); the next success closes it.
func (f *Fetcher) FetchSource(ctx context.Context, sourceID string) (importer.Result, error) {
	q := db.New(f.Pool)
	src, err := q.GetSourceFetchConfig(ctx, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return importer.Result{}, fmt.Errorf("jobs: unknown source %q", sourceID)
	}
	if err != nil {
		return importer.Result{}, err
	}
	var cfg fetch.Config
	if err := json.Unmarshal(src.FetchConfig, &cfg); err != nil {
		return importer.Result{}, fmt.Errorf("jobs: fetch settings of %q: %w", sourceID, err)
	}
	if !cfg.Enabled() {
		return importer.Result{}, ErrNothingToFetch
	}

	res, err := f.fetchAndImport(ctx, q, sourceID, cfg)
	at := f.now()
	if err != nil {
		message := err.Error()
		if dbErr := q.SetSourceFetchResult(ctx, db.SetSourceFetchResultParams{ID: sourceID, LastFetchAt: &at, LastFetchError: &message}); dbErr != nil {
			return res, errors.Join(err, dbErr)
		}
		open, dbErr := q.HasOpenSourceTask(ctx, db.HasOpenSourceTaskParams{SourceID: sourceID, Kind: "fetch_failed"})
		if dbErr != nil {
			return res, errors.Join(err, dbErr)
		}
		if !open {
			dbErr = q.InsertSourceTask(ctx, db.InsertSourceTaskParams{Kind: "fetch_failed",
				Title: "Abholen fehlgeschlagen: " + src.Name, Detail: message, Priority: 1, SourceID: &sourceID})
		}
		return res, errors.Join(err, dbErr)
	}
	if err := q.SetSourceFetchResult(ctx, db.SetSourceFetchResultParams{ID: sourceID, LastFetchAt: &at}); err != nil {
		return res, err
	}
	return res, q.CloseSourceTasks(ctx, db.CloseSourceTasksParams{SourceID: sourceID, Kind: "fetch_failed", Note: "Abholen inzwischen erfolgreich"})
}

func (f *Fetcher) fetchAndImport(ctx context.Context, q *db.Queries, sourceID string, cfg fetch.Config) (importer.Result, error) {
	getenv := f.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	data, err := fetch.Get(ctx, cfg, f.HTTP, getenv)
	if err != nil {
		return importer.Result{}, err
	}
	conn, err := importer.ConnectorFor(ctx, q, sourceID)
	if err != nil {
		return importer.Result{}, err
	}
	return importer.Run(ctx, f.Pool, conn, bytes.NewReader(data), importer.Options{RawDir: f.RawDir, Now: f.Now})
}

// DueSources returns the sources whose scheduled fetch is due.
func DueSources(ctx context.Context, q *db.Queries, now time.Time) ([]string, error) {
	rows, err := q.ListSourceFetchConfigs(ctx)
	if err != nil {
		return nil, err
	}
	var due []string
	for _, row := range rows {
		var cfg fetch.Config
		if err := json.Unmarshal(row.FetchConfig, &cfg); err != nil || !cfg.Enabled() || cfg.IntervalMinutes <= 0 {
			continue
		}
		interval := time.Duration(cfg.IntervalMinutes) * time.Minute
		if row.LastFetchAt == nil || !now.Before(row.LastFetchAt.Add(interval)) {
			due = append(due, row.ID)
		}
	}
	return due, nil
}
