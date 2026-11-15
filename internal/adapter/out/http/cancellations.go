package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/port/out"
)

const (
	cancelPath   = "/api/v1/sagas/internal/tenants/%s/cancel-departure"
	maxKeyLength = 128 // the most an Idempotency-Key may have (Annex C)
)

// Cancellations starts the saga cancel-departure on the internal route that only job 3 may call.
type Cancellations struct {
	client *httpclient.Client
}

var _ out.Cancellations = (*Cancellations)(nil)

// NewCancellations returns the adapter over a client whose base URL is the saga cancel-departure.
func NewCancellations(client *httpclient.Client) *Cancellations {
	return &Cancellations{client: client}
}

// Start asks the saga to cancel one departure of the tenant. A 201 is a saga started and a 200 the saga the key already started: both
// are a success, and the saga keeps its outcome in its own state, so the answer is not read. The key also lets the runtime repeat
// the POST after a network failure, a 429 or a 5xx.
func (c *Cancellations) Start(ctx context.Context, tenantID, departureCode, idempotencyKey string) error {
	if !uuid.MatchString(tenantID) || departureCode == "" || len(idempotencyKey) < minKeyLength || len(idempotencyKey) > maxKeyLength {
		return fmt.Errorf("a tenant uuid, a departure code and a key of %d to %d characters are required", minKeyLength, maxKeyLength)
	}
	err := c.client.Do(ctx, httpclient.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf(cancelPath, tenantID),
		Body: struct {
			DepartureCode string `json:"departureCode"`
		}{departureCode},
		IdempotencyKey: idempotencyKey,
	}, nil)
	if err != nil {
		return fmt.Errorf("start the saga cancel-departure: %w", err)
	}
	return nil
}
