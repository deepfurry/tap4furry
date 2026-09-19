// Package jobs is the sole River infrastructure boundary.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	platformruntime "github.com/deepfurry/tap4furry/server/internal/runtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type Worker struct{ client *river.Client[pgx.Tx] }

// River v0.47.0 rejects an empty registry at Start. This infrastructure-only job
// proves actual execution in smoke checks; it is never scheduled at normal startup.
type probeArgs struct{}

func (probeArgs) Kind() string { return "infrastructure.probe.v1" }

type probeWorker struct {
	river.WorkerDefaults[probeArgs]
	pool *pgxpool.Pool
}

func (w *probeWorker) Work(ctx context.Context, _ *river.Job[probeArgs]) error {
	return database.Ready(ctx, w.pool)
}

func New(pool *pgxpool.Pool, schema string, logger *slog.Logger) (*Worker, error) {
	if schema != "river" {
		return nil, errors.New("River schema must be river")
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &probeWorker{pool: pool})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: schema, Workers: workers,
		Queues:     map[string]river.QueueConfig{"default": {MaxWorkers: 1}},
		Logger:     platformruntime.DependencyLogger(logger, "river"),
		JobTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, database.SafeError("River construction", err)
	}
	return &Worker{client: client}, nil
}

func (w *Worker) Start(ctx context.Context) error {
	if err := w.client.Start(ctx); err != nil {
		return database.SafeError("River start", err)
	}
	return nil
}

func (w *Worker) Stop(ctx context.Context) error {
	if err := w.client.Stop(ctx); err != nil {
		return database.SafeError("River stop", err)
	}
	return nil
}

// Probe waits for durable completion, then deletes only the job it inserted.
func (w *Worker) Probe(ctx context.Context) error {
	inserted, err := w.client.Insert(ctx, probeArgs{}, &river.InsertOpts{MaxAttempts: 1})
	if err != nil {
		return database.SafeError("River probe enqueue", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("River probe did not complete before deadline")
		case <-ticker.C:
			job, err := w.client.JobGet(ctx, inserted.Job.ID)
			if err != nil {
				return database.SafeError("River probe read", err)
			}
			switch job.State {
			case "completed":
				if _, err := w.client.JobDelete(ctx, job.ID); err != nil {
					return database.SafeError("River probe cleanup", err)
				}
				return nil
			case "discarded", "cancelled":
				return errors.New("River probe failed (job details withheld)")
			}
		}
	}
}
