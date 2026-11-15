package usecase

import (
	"context"
	"fmt"
	"log/slog"

	workerruntime "github.com/code-corhuila/multi-tour-worker-runtime"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

// MaxDueLimit is the most the due list of operations-cost gives in one ask (its limit is 1 to 100).
const MaxDueLimit = 100

// StartInactiveCancellations starts the saga cancel-departure for the departures of an inactive tenant whose start arrived (HU-WRK-003
// AC2, PDR I1). It decides nothing: identity-audit says which tenants are inactive, operations-cost which departures are due, and the
// saga cancels. The retries of a network failure, a 429 or a 5xx happen in the client of the runtime, never for a 4xx.
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

// Run asks each inactive tenant once for at most the batch of its due departures and starts one saga per departure. A tenant or a
// departure that fails is counted and logged and the run goes on; what is left waits for the next run.
func (j *StartInactiveCancellations) Run(ctx context.Context, batch int) (processed, failed int) {
	tenants, err := j.tenants.ListInactive(ctx)
	if err != nil {
		j.log.WarnContext(ctx, "the inactive tenants could not be listed, the next run asks again", "error", err)
		return 0, 1
	}
	// shortcut: one ask per tenant (oldest first, at most 100). A departure whose saga ended FAILED or COMPENSATED stays
	// SCHEDULED and keeps its place, so a tenant with a full batch of those never reaches its newer ones; asking another page
	// does not help: the fix is for operations-cost to leave out the departures whose cancellation already failed.
	limit := min(batch, MaxDueLimit)
	for _, tenant := range tenants {
		if ctx.Err() != nil {
			break
		}
		codes, err := j.departures.DueToStart(ctx, tenant, limit)
		if err != nil {
			failed++
			j.log.WarnContext(ctx, "the due departures could not be listed, the next run asks again", "tenantId", tenant, "error", err)
			continue
		}
		done, bad := workerruntime.Each(ctx, j.log, codes, func(ctx context.Context, code string) error { return j.start(ctx, tenant, code) })
		processed, failed = processed+done, failed+bad
	}
	j.log.InfoContext(ctx, "cancellations of inactive tenants started", "tenants", len(tenants), "started", processed, "failed", failed)
	return processed, failed
}

// start names the tenant and the departure in the key, never the run: the next run asks the same saga and it answers 200.
func (j *StartInactiveCancellations) start(ctx context.Context, tenant, code string) error {
	if err := j.cancellations.Start(ctx, tenant, code, fmt.Sprintf("deadlines-cancel:%s:%s", tenant, code)); err != nil {
		return fmt.Errorf("start the cancellation of departure %s of tenant %s: %w", code, tenant, err)
	}
	j.log.InfoContext(ctx, "cancellation started", "tenantId", tenant, "departureCode", code)
	return nil
}
