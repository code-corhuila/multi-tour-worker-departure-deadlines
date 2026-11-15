package http

import (
	"context"
	"fmt"

	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

const (
	inactiveTenantsPath = "/api/v1/identity-audit/internal/tenants?status=INACTIVE&page=%d&limit=%d"
	tenantsPage         = 100
	maxPages            = 1000 // a guard against a service that never stops paging
)

// Tenants reads the inactive tenants from the internal route of identity-audit, which answers each one with its id and its status.
type Tenants struct {
	client *httpclient.Client
}

var _ out.Tenants = (*Tenants)(nil)

// NewTenants returns the adapter over a client whose base URL is identity-audit.
func NewTenants(client *httpclient.Client) *Tenants { return &Tenants{client: client} }

type tenantItem struct {
	TenantID string `json:"tenantId"`
	Status   string `json:"status"`
}

// ListInactive reads every page of GET .../internal/tenants?status=INACTIVE. An id that is not a UUID or a tenant that is not inactive is
// an error, never a skipped item: the job would start the cancellation of the departures of whatever tenant came back.
func (t *Tenants) ListInactive(ctx context.Context) ([]string, error) {
	var ids []string
	for page := 1; page <= maxPages; page++ {
		got, err := httpclient.List[tenantItem](ctx, t.client, fmt.Sprintf(inactiveTenantsPath, page, tenantsPage))
		if err != nil {
			return nil, fmt.Errorf("list the inactive tenants: %w", err)
		}
		for _, item := range got.Data {
			if !uuid.MatchString(item.TenantID) {
				return nil, fmt.Errorf("list the inactive tenants: an item has no tenant uuid")
			}
			if item.Status != "INACTIVE" {
				return nil, fmt.Errorf("list the inactive tenants: an item is not inactive")
			}
			ids = append(ids, item.TenantID)
		}
		if page >= got.Meta.TotalPages {
			return ids, nil
		}
	}
	return nil, fmt.Errorf("list the inactive tenants: more than %d pages", maxPages)
}
