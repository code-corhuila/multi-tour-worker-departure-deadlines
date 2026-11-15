package http_test

import (
	"net/http"
	"strings"
	"testing"

	deadlineshttp "github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/adapter/out/http"
)

const cancelKey = "deadlines-cancel:" + tenantOne + ":SAL-001"

// I·AC-WK-02.1, -02.2: a POST to the internal route of the saga for the tenant of the path, with the departure in the body, the key of
// the use case and the credential; a saga started (201) and the saga the key already started (200) are both a success.
func TestCancellations_starts_the_saga_of_one_departure(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		srv, seen := newService(t, `{"id":"x"}`, status)

		err := deadlineshttp.NewCancellations(newClient(srv.URL)).Start(ctx(), tenantOne, "SAL-001", cancelKey)

		if err != nil {
			t.Fatalf("status %d: err = %v", status, err)
		}
		if seen.method != http.MethodPost || seen.path != "/api/v1/sagas/internal/tenants/"+tenantOne+"/cancel-departure" || seen.query != "" {
			t.Errorf("call = %s %s?%s", seen.method, seen.path, seen.query)
		}
		if seen.sent != `{"departureCode":"SAL-001"}` || seen.header.Get("Content-Type") != "application/json" {
			t.Errorf("body = %q, content type %q", seen.sent, seen.header.Get("Content-Type"))
		}
		if seen.header.Get("Idempotency-Key") != cancelKey || seen.header.Get("Authorization") != "Bearer "+token ||
			seen.header.Get("X-Correlation-Id") != "run-1234" {
			t.Errorf("headers = %v", seen.header)
		}
	}
}

// AC-WK-02.4: a 5xx is repeated with the same key; a 4xx is not, and its error never carries the token.
func TestCancellations_retries_a_5xx_with_the_same_key_but_not_a_4xx(t *testing.T) {
	srv, seen := newService(t, `{"id":"x"}`, http.StatusServiceUnavailable, http.StatusCreated)
	err := deadlineshttp.NewCancellations(newClient(srv.URL)).Start(ctx(), tenantOne, "SAL-001", cancelKey)
	if err != nil || seen.calls.Load() != 2 || seen.header.Get("Idempotency-Key") != cancelKey {
		t.Errorf("err=%v calls=%d key=%q, want no error after 2 calls with the same key", err, seen.calls.Load(), seen.header.Get("Idempotency-Key"))
	}

	srv, seen = newService(t, ``, http.StatusNotFound, http.StatusNotFound)
	err = deadlineshttp.NewCancellations(newClient(srv.URL)).Start(ctx(), tenantOne, "SAL-001", cancelKey)
	if err == nil || seen.calls.Load() != 1 || strings.Contains(err.Error(), token) {
		t.Errorf("err=%v calls=%d, want an error without the token after one call", err, seen.calls.Load())
	}
}

func TestCancellations_refuses_a_bad_tenant_code_or_key_without_calling(t *testing.T) {
	srv, seen := newService(t, `{}`)
	a := deadlineshttp.NewCancellations(newClient(srv.URL))
	for _, c := range [][3]string{{"../x", "SAL-001", cancelKey}, {tenantOne, "", cancelKey}, {tenantOne, "SAL-001", "short"},
		{tenantOne, "SAL-001", strings.Repeat("k", 129)}} {
		if err := a.Start(ctx(), c[0], c[1], c[2]); err == nil {
			t.Errorf("%q accepted", c)
		}
	}
	if seen.calls.Load() != 0 {
		t.Errorf("calls = %d, want none", seen.calls.Load())
	}
}
