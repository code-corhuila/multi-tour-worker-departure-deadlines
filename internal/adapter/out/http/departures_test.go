package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-worker-runtime/config"
	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	deadlineshttp "github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/adapter/out/http"
)

const token = "s3cret-service-token"

// fakeService answers each call with the next scripted status and body, and remembers what it saw.
type fakeService struct {
	statuses []int
	body     string
	calls    atomic.Int32
	method   string
	path     string
	query    string
	header   http.Header
}

func newService(t *testing.T, body string, statuses ...int) (*httptest.Server, *fakeService) {
	t.Helper()
	f := &fakeService{statuses: statuses, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(f.calls.Add(1))
		f.method, f.path, f.query, f.header = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		status := http.StatusOK
		if n <= len(f.statuses) {
			status = f.statuses[n-1]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(f.body))
			return
		}
		_, _ = w.Write([]byte(`{"error":"X","message":"m","traceId":"t"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

// newClient never really sleeps between attempts.
func newClient(url string) *httpclient.Client {
	return httpclient.New(url, config.Secret(token),
		httpclient.WithSleeper(func(context.Context, time.Duration) error { return nil }))
}

func ctx() context.Context { return correlation.WithID(context.Background(), "run-1234") }

// AC-WK-01.1: a POST to the internal route with the limit, the credential and the correlation id; the count is the answer.
func TestDepartures_marks_through_the_internal_route_and_reads_the_count(t *testing.T) {
	srv, seen := newService(t, `{"marked":7}`)

	marked, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 100)

	if err != nil || marked != 7 {
		t.Fatalf("marked=%d err=%v, want 7 and no error", marked, err)
	}
	if seen.method != http.MethodPost || seen.path != "/api/v1/operations-cost/internal/departures/pending-regularization" || seen.query != "limit=100" {
		t.Errorf("call = %s %s?%s", seen.method, seen.path, seen.query)
	}
	if seen.header.Get("Authorization") != "Bearer "+token || seen.header.Get("X-Correlation-Id") != "run-1234" {
		t.Errorf("headers = %v", seen.header)
	}
}

// AC-WK-01.3: network, 429 and 5xx are retried a bounded number of times; a 4xx is not.
func TestDepartures_retries_a_5xx_and_a_429_but_not_a_4xx(t *testing.T) {
	srv, seen := newService(t, `{"marked":2}`, http.StatusServiceUnavailable, http.StatusTooManyRequests)

	marked, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 10)

	if err != nil || marked != 2 || seen.calls.Load() != 3 {
		t.Errorf("marked=%d err=%v calls=%d, want 2, no error and 3 calls", marked, err, seen.calls.Load())
	}

	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusUnauthorized} {
		srv, seen := newService(t, `{"marked":2}`, status, status, status)
		if _, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 10); err == nil || seen.calls.Load() != 1 {
			t.Errorf("status %d: err=%v calls=%d, want an error after one call", status, err, seen.calls.Load())
		}
	}
}

func TestDepartures_gives_up_after_the_attempts_and_says_so(t *testing.T) {
	srv, seen := newService(t, ``, 503, 503, 503, 503)

	_, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 10)

	if err == nil || seen.calls.Load() != 3 {
		t.Errorf("err=%v calls=%d, want an error after 3 calls", err, seen.calls.Load())
	}
}

// What the service answers is not trusted: a count that is not a number, negative or above the limit is an error.
func TestDepartures_refuses_an_answer_that_is_not_what_the_contract_says(t *testing.T) {
	for _, body := range []string{`{}`, `{"marked":-1}`, `{"marked":11}`, `{"marked":"2"}`, `not json`} {
		srv, _ := newService(t, body)
		if _, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 10); err == nil {
			t.Errorf("answer %q accepted, want an error", body)
		}
	}
}

// The credential never reaches an error message.
func TestDepartures_an_error_never_carries_the_token(t *testing.T) {
	srv, _ := newService(t, ``, 403)

	_, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 10)

	if err == nil || strings.Contains(err.Error(), token) {
		t.Errorf("err = %v", err)
	}
}

func TestDepartures_refuses_a_limit_that_is_not_positive(t *testing.T) {
	srv, seen := newService(t, `{"marked":0}`)
	if _, err := deadlineshttp.NewDepartures(newClient(srv.URL)).MarkPendingRegularization(ctx(), 0); err == nil || seen.calls.Load() != 0 {
		t.Errorf("err=%v calls=%d, want an error and no call", err, seen.calls.Load())
	}
}
