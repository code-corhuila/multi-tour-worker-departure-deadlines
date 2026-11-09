// Package out holds the ports the job needs from the outside world.
package out

import "context"

// Departures is operations-cost as the job sees it: the application never learns that it is HTTP.
type Departures interface {
	// MarkPendingRegularization asks the service to mark, across tenants, up to limit departures past their estimated end, and
	// says how many it marked. Safe to repeat: the service marks by state.
	MarkPendingRegularization(ctx context.Context, limit int) (marked int, err error)
}
