// Package http is the output adapter that calls operations-cost on its internal route.
package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

const markPath = "/api/v1/operations-cost/internal/departures/pending-regularization"

// minKeyLength is the least an Idempotency-Key may have (Annex C); a shorter run id is padded by a fixed prefix.
const minKeyLength = 8

// Departures calls operations-cost through the client of the runtime.
type Departures struct {
	client *httpclient.Client
}

var _ out.Departures = (*Departures)(nil)

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

func key(ctx context.Context) string {
	id := correlation.From(ctx)
	if len(id) < minKeyLength {
		return "deadlines-" + id
	}
	return id
}
