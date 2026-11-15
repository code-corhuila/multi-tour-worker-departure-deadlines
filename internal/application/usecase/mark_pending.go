// Package usecase holds the jobs of the worker.
package usecase

import (
	"context"
	"log/slog"

	workerruntime "github.com/code-corhuila/multi-tour-worker-runtime"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

// MaxAsks bounds a run: more than this many full batches wait for the next run (Annex D, bounded run).
const MaxAsks = 5

// MarkPending is the job that asks operations-cost to mark the departures past their estimated end (HU-WRK-003, HU-OPS-003 part B).
// It decides nothing about a departure: the rule and the data belong to the service, and time never closes a departure.
type MarkPending struct {
	departures out.Departures
	log        *slog.Logger
}

var _ workerruntime.Job = (*MarkPending)(nil)

// NewMarkPending wires the job.
func NewMarkPending(departures out.Departures, log *slog.Logger) *MarkPending {
	return &MarkPending{departures: departures, log: log}
}

// Name is how the runner logs the job.
func (*MarkPending) Name() string { return "departure-deadlines" }

// Run asks with the batch as the limit and asks again while the batch comes back full, up to MaxAsks. A failed ask ends the run
// counted; the next run asks again, because the service marks by state and nothing is lost.
func (j *MarkPending) Run(ctx context.Context, batch int) (processed, failed int) {
	asks := 0
	for asks < MaxAsks && ctx.Err() == nil {
		asks++
		marked, err := j.departures.MarkPendingRegularization(ctx, batch)
		if err != nil {
			failed++
			j.log.WarnContext(ctx, "the departures could not be marked, the next run asks again", "error", err, "asks", asks)
			break
		}
		processed += marked
		if marked < batch {
			break
		}
	}
	j.log.InfoContext(ctx, "departures past their estimated end marked", "marked", processed, "asks", asks, "failed", failed)
	return processed, failed
}
