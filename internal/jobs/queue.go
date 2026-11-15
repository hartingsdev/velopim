package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"

	"velopim/internal/db"
	"velopim/internal/shopify"
	"velopim/internal/shopsync"
)

// onePending allows one job per kind and arguments that is waiting or
// running. A finished job does not block the next one: River's default also
// counts completed jobs, which would allow each fetch only once until the
// finished job is cleaned up a day later.
var onePending = river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
	rivertype.JobStateRetryable, rivertype.JobStateScheduled,
}}

// FetchSourceArgs fetches and imports one source's file.
type FetchSourceArgs struct {
	SourceID string `json:"source_id"`
}

func (FetchSourceArgs) Kind() string { return "fetch_source" }

// InsertOpts: one job per source at a time, and no automatic retries. A
// failed fetch is tried again at the next scheduled time; retrying at once
// would only repeat the import of a broken file.
func (FetchSourceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: onePending}
}

// ScheduleArgs looks for sources whose fetch is due and enqueues them.
type ScheduleArgs struct{}

func (ScheduleArgs) Kind() string { return "schedule_fetches" }

func (ScheduleArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: onePending}
}

// SyncArgs sends pending changes to Shopify.
type SyncArgs struct{}

func (SyncArgs) Kind() string { return "shopify_sync" }

// InsertOpts: sync jobs are deliberately not unique. A change approved
// while a sync is running must get a sync of its own; a unique job would be
// dropped as "already running" and the change would wait for the next
// trigger. A sync with nothing to send only reads the plan and is cheap.
func (SyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

// syncDelay lets a burst of approvals or a whole import finish before the
// sync starts.
const syncDelay = 5 * time.Second

type fetchWorker struct {
	river.WorkerDefaults[FetchSourceArgs]
	fetcher *Fetcher
	queue   *Queue
}

func (w *fetchWorker) Timeout(*river.Job[FetchSourceArgs]) time.Duration { return 30 * time.Minute }

func (w *fetchWorker) Work(ctx context.Context, job *river.Job[FetchSourceArgs]) error {
	res, err := w.fetcher.FetchSource(ctx, job.Args.SourceID)
	if errors.Is(err, ErrNothingToFetch) {
		return nil // the address was removed after the job was queued
	}
	if err != nil {
		return err
	}
	// What the import changed without approval, e.g. availability, goes to
	// Shopify right away.
	if err := w.queue.EnqueueSync(ctx); err != nil {
		return err
	}
	log.Printf("fetched %s: run %d, %d articles, %d new, %d changed, %d for approval",
		job.Args.SourceID, res.RunID, res.Records, res.New, res.Changed, res.PendingApproval)
	return nil
}

type scheduleWorker struct {
	river.WorkerDefaults[ScheduleArgs]
	pool  *pgxpool.Pool
	queue *Queue
}

func (w *scheduleWorker) Work(ctx context.Context, _ *river.Job[ScheduleArgs]) error {
	due, err := DueSources(ctx, db.New(w.pool), time.Now())
	if err != nil {
		return err
	}
	for _, id := range due {
		if err := w.queue.EnqueueFetch(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

type syncWorker struct {
	river.WorkerDefaults[SyncArgs]
	pool  *pgxpool.Pool
	store shopsync.Store
}

func (w *syncWorker) Timeout(*river.Job[SyncArgs]) time.Duration { return 30 * time.Minute }

func (w *syncWorker) Work(ctx context.Context, _ *river.Job[SyncArgs]) error {
	res, err := shopsync.Run(ctx, w.pool, w.store, time.Now())
	if errors.Is(err, shopify.ErrWritesDisabled) {
		return nil // switched off after the job was queued
	}
	if err != nil {
		return err
	}
	if res.Synced > 0 || res.Failed > 0 {
		log.Printf("shopify sync: %d synced, %d failed", res.Synced, res.Failed)
	}
	return nil
}

// Config configures the queue.
type Config struct {
	Fetcher *Fetcher
	// Store is the Shopify client; nil disables the sync job.
	Store shopsync.Store
	// SyncOnChange queues a sync after imports, approvals and edits. Set it
	// only when writing to Shopify is enabled.
	SyncOnChange bool
	// SyncInterval runs the Shopify sync periodically; zero means only on
	// request.
	SyncInterval time.Duration
}

// Queue runs the background jobs.
type Queue struct {
	client *river.Client[pgx.Tx]
	// syncOnChange says whether EnqueueSync does anything.
	syncOnChange bool
}

// Migrate creates or updates River's own tables.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

// New prepares the queue. Call Start to begin working jobs.
func New(pool *pgxpool.Pool, cfg Config) (*Queue, error) {
	q := &Queue{syncOnChange: cfg.SyncOnChange && cfg.Store != nil}
	workers := river.NewWorkers()
	river.AddWorker(workers, &fetchWorker{fetcher: cfg.Fetcher, queue: q})
	river.AddWorker(workers, &scheduleWorker{pool: pool, queue: q})

	// Every minute: look for sources whose fetch is due.
	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return ScheduleArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
	}
	if cfg.Store != nil {
		river.AddWorker(workers, &syncWorker{pool: pool, store: cfg.Store})
		if cfg.SyncInterval > 0 {
			periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(cfg.SyncInterval),
				func() (river.JobArgs, *river.InsertOpts) { return SyncArgs{}, nil }, nil))
		}
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		// Imports and syncs are heavy and touch the same tables: one at a time.
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:      workers,
		PeriodicJobs: periodic,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: %w", err)
	}
	q.client = client
	return q, nil
}

// Start begins working jobs until ctx ends.
func (q *Queue) Start(ctx context.Context) error { return q.client.Start(ctx) }

// Stop waits for the running job and stops.
func (q *Queue) Stop(ctx context.Context) error { return q.client.Stop(ctx) }

// EnqueueFetch queues a fetch of the source. A fetch of the same source
// that is already waiting or running is not queued twice.
func (q *Queue) EnqueueFetch(ctx context.Context, sourceID string) error {
	_, err := q.client.Insert(ctx, FetchSourceArgs{SourceID: sourceID}, nil)
	return err
}

// EnqueueSync queues a Shopify sync a few seconds from now. It does nothing
// when no store is configured or writing is switched off: then the sync
// stays a dry run in the interface.
func (q *Queue) EnqueueSync(ctx context.Context) error {
	if !q.syncOnChange {
		return nil
	}
	_, err := q.client.Insert(ctx, SyncArgs{}, &river.InsertOpts{ScheduledAt: time.Now().Add(syncDelay)})
	return err
}
