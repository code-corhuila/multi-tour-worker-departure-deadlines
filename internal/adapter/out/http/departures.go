// Package http is the output adapter that calls operations-cost, identity-audit and the saga cancel-departure on their internal routes.
package http

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

const (
	markPath       = "/api/v1/operations-cost/internal/departures/pending-regularization"
	dueToStartPath = "/api/v1/operations-cost/internal/tenants/%s/departures/due-to-start?limit=%d"
)

var (
	// uuid guards every tenant id that goes into a path: anything else could reach another route.
	uuid          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	departureCode = regexp.MustCompile(`^SAL-[0-9]{3,6}$`) // as the contract of operations-cost 0.16.0 answers it
)

// minKeyLength is the least an Idempotency-Key may have (Annex C); a shorter run id is padded by a fixed prefix.
const minKeyLength = 8

// Departures calls operations-cost through the client of the runtime.
type Departures struct {
	client *httpclient.Client
}

var (
	_ out.Departures    = (*Departures)(nil)
	_ out.DueDepartures = (*Departures)(nil)
)

// NewDepartures returns the adapter over a client whose base URL is operations-cost.
func NewDepartures(client *httpclient.Client) *Departures { return &Departures{client: client} }

// MarkPendingRegularization is a POST that changes nothing twice, because the service marks by state. The runtime repeats a POST
// after a network failure, a 429 or a 5xx only when it carries an Idempotency-Key, so the call carries the id of the run: the
// service ignores it, and it is what lets a transient failure be repeated.
func (d *Departures) MarkPendingRegularization(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, fmt.Errorf("the limit of an ask must be positive, got %d", limit)
	}
	var answer struct {
		Marked *int `json:"marked"`
	}
	err := d.client.Do(ctx, httpclient.Request{
		Method:         http.MethodPost,
		Path:           fmt.Sprintf("%s?limit=%d", markPath, limit),
		IdempotencyKey: key(ctx),
	}, &answer)
	if err != nil {
		return 0, fmt.Errorf("mark the departures: %w", err)
	}
	if answer.Marked == nil || *answer.Marked < 0 || *answer.Marked > limit {
		return 0, fmt.Errorf("mark the departures: the answer is not a count from 0 to %d", limit)
	}
	return *answer.Marked, nil
}

// DueToStart reads the due list of one tenant. More codes than the limit, or a code that is not one, is an error and never a partial
// list: the saga would cancel whatever came back.
func (d *Departures) DueToStart(ctx context.Context, tenantID string, limit int) ([]string, error) {
	if !uuid.MatchString(tenantID) || limit < 1 {
		return nil, fmt.Errorf("a tenant uuid and a positive limit are required, got limit %d", limit)
	}
	var answer struct {
		Data *[]struct {
			DepartureCode string `json:"departureCode"`
		} `json:"data"`
	}
	if err := d.client.Get(ctx, fmt.Sprintf(dueToStartPath, tenantID, limit), &answer); err != nil {
		return nil, fmt.Errorf("list the due departures: %w", err)
	}
	if answer.Data == nil || len(*answer.Data) > limit {
		return nil, fmt.Errorf("list the due departures: the answer is not a list of at most %d", limit)
	}
	codes := make([]string, 0, len(*answer.Data))
	for _, item := range *answer.Data {
		if !departureCode.MatchString(item.DepartureCode) {
			return nil, fmt.Errorf("list the due departures: an item has no departure code")
		}
		codes = append(codes, item.DepartureCode)
	}
	return codes, nil
}

func key(ctx context.Context) string {
	id := correlation.From(ctx)
	if len(id) < minKeyLength {
		return "deadlines-" + id
	}
	return id
}
