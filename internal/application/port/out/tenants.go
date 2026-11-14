package out

import "context"

// Tenants is identity-audit as the job sees it.
type Tenants interface {
	// ListInactive reads every page of the inactive tenants and gives their ids. A failure is an error, never a partial list.
	ListInactive(ctx context.Context) ([]string, error)
}
