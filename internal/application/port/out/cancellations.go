package out

import "context"

// DueDepartures is the second question the job asks operations-cost: the departures of one tenant that should have started.
type DueDepartures interface {
	// DueToStart gives up to limit codes of the SCHEDULED departures of the tenant whose scheduled start passed, oldest first.
	// It does not look at the state of the tenant.
	DueToStart(ctx context.Context, tenantID string, limit int) ([]string, error)
}

// Cancellations is the saga cancel-departure as the job sees it.
type Cancellations interface {
	// Start starts the cancellation of one departure of the tenant. The same key answers the saga already started, so a repeat
	// cancels nothing twice.
	Start(ctx context.Context, tenantID, departureCode, idempotencyKey string) error
}
