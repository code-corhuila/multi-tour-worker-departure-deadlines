package usecase

import (
	"context"
	"log/slog"

	workerruntime "github.com/code-corhuila/multi-tour-worker-runtime"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

// MaxDueLimit is the most the due list of operations-cost gives in one ask (its limit is 1 to 100).
const MaxDueLimit = 100

// StartInactiveCancellations starts the saga cancel-departure for the departures of an inactive tenant whose start arrived.
type StartInactiveCancellations struct {
	tenants       out.Tenants
	departures    out.DueDepartures
	cancellations out.Cancellations
	log           *slog.Logger
}

var _ workerruntime.Job = (*StartInactiveCancellations)(nil)

// NewStartInactiveCancellations wires the job.
func NewStartInactiveCancellations(tenants out.Tenants, departures out.DueDepartures, cancellations out.Cancellations,
	log *slog.Logger) *StartInactiveCancellations {
	return &StartInactiveCancellations{tenants: tenants, departures: departures, cancellations: cancellations, log: log}
}

// Name is how the runner logs the job.
func (*StartInactiveCancellations) Name() string { return "inactive-tenant-cancellations" }

// Run is not written yet.
func (j *StartInactiveCancellations) Run(ctx context.Context, batch int) (processed, failed int) {
	return 0, 0
}
