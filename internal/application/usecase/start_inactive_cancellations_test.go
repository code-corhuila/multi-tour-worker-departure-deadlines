package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/code-corhuila/multi-tour-worker-runtime/logging"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/usecase"
)

const (
	inactive = "8f2b4c1e-0a7d-4e3b-9c55-1d2e3f4a5b6c"
	other    = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	third    = "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b"
)

// fakeTenants answers the inactive tenants it was given, or its error.
type fakeTenants struct {
	ids []string
	err error
}

func (f *fakeTenants) ListInactive(context.Context) ([]string, error) { return f.ids, f.err }

// fakeDue answers the due departures of each tenant (or its error) and remembers each ask.
type fakeDue struct {
	byTenant map[string][]string
	errs     map[string]error
	asked    []string
	limits   []int
}

func (f *fakeDue) DueToStart(_ context.Context, tenantID string, limit int) ([]string, error) {
	f.asked, f.limits = append(f.asked, tenantID), append(f.limits, limit)
	if err := f.errs[tenantID]; err != nil {
		return nil, err
	}
	codes := f.byTenant[tenantID]
	return codes[:min(limit, len(codes))], nil
}

type start struct{ tenant, code, key string }

// fakeCancellations remembers each start; a code in fail answers an error, and onStart runs after each one.
type fakeCancellations struct {
	starts  []start
	fail    map[string]bool
	onStart func()
}

func (f *fakeCancellations) Start(_ context.Context, tenantID, code, key string) error {
	f.starts = append(f.starts, start{tenantID, code, key})
	if f.onStart != nil {
		f.onStart()
	}
	if f.fail[code] {
		return errors.New("saga unavailable after the retries")
	}
	return nil
}

func cancellations(t *fakeTenants, d *fakeDue, c *fakeCancellations, out *bytes.Buffer) *usecase.StartInactiveCancellations {
	return usecase.NewStartInactiveCancellations(t, d, c, logging.New(out))
}

// RN-WK-020, AC-WK-02.1 and AC-WK-02.3: only the tenants identity-audit lists as inactive are asked, and each departure it answers
// starts one saga for that tenant; the departures of any other tenant are never asked for.
func TestA_only_the_departures_of_an_inactive_tenant_start_the_saga(t *testing.T) {
	due := &fakeDue{byTenant: map[string][]string{inactive: {"DEP-001", "DEP-002"}, other: {"DEP-900"}}}
	c := &fakeCancellations{}

	processed, failed := cancellations(&fakeTenants{ids: []string{inactive}}, due, c, &bytes.Buffer{}).Run(context.Background(), 100)

	if processed != 2 || failed != 0 {
		t.Errorf("processed=%d failed=%d, want 2 and 0", processed, failed)
	}
	if !slices.Equal(due.asked, []string{inactive}) {
		t.Errorf("asked the due departures of %v, want only the inactive tenant", due.asked)
	}
	if len(c.starts) != 2 || c.starts[0].tenant != inactive || c.starts[0].code != "DEP-001" || c.starts[1].code != "DEP-002" {
		t.Errorf("starts = %v, want DEP-001 and DEP-002 of the inactive tenant", c.starts)
	}
}

func TestA_no_inactive_tenant_starts_nothing(t *testing.T) {
	due := &fakeDue{byTenant: map[string][]string{other: {"DEP-900"}}}
	c := &fakeCancellations{}

	processed, failed := cancellations(&fakeTenants{}, due, c, &bytes.Buffer{}).Run(context.Background(), 100)

	if processed != 0 || failed != 0 || len(due.asked) != 0 || len(c.starts) != 0 {
		t.Errorf("processed=%d failed=%d asked=%v starts=%v, want nothing", processed, failed, due.asked, c.starts)
	}
}

// RN-WK-021, AC-WK-02.2: the key names the tenant and the departure, never the run, so the next run asks the saga it started.
func TestA_the_same_departure_carries_the_same_key_in_two_runs(t *testing.T) {
	due := &fakeDue{byTenant: map[string][]string{inactive: {"DEP-001"}}}
	c := &fakeCancellations{}
	job := cancellations(&fakeTenants{ids: []string{inactive}}, due, c, &bytes.Buffer{})

	job.Run(context.Background(), 100)
	job.Run(context.Background(), 100)

	want := "deadlines-cancel:" + inactive + ":DEP-001"
	if len(c.starts) != 2 || c.starts[0].key != want || c.starts[1].key != want {
		t.Errorf("starts = %v, want two starts with the key %s", c.starts, want)
	}
}

// RN-WK-022, AC-WK-02.4: a tenant whose list fails and a departure whose start fails are counted and logged, and the run goes on.
func TestA_a_failed_tenant_does_not_stop_the_others(t *testing.T) {
	due := &fakeDue{
		byTenant: map[string][]string{other: {"DEP-010", "DEP-011"}, third: {"DEP-020"}},
		errs:     map[string]error{inactive: errors.New("operations-cost unavailable after the retries")},
	}
	c := &fakeCancellations{fail: map[string]bool{"DEP-010": true}}
	var out bytes.Buffer

	processed, failed := cancellations(&fakeTenants{ids: []string{inactive, other, third}}, due, c, &out).Run(context.Background(), 100)

	if processed != 2 || failed != 2 {
		t.Errorf("processed=%d failed=%d, want 2 started (DEP-011, DEP-020) and 2 failed (the list of one tenant, DEP-010)", processed, failed)
	}
	if !slices.Equal(due.asked, []string{inactive, other, third}) {
		t.Errorf("asked %v, want the three tenants", due.asked)
	}
	if !strings.Contains(out.String(), inactive) {
		t.Errorf("the failed tenant should be logged: %s", out.String())
	}
}

func TestA_a_list_of_tenants_that_fails_is_one_failure(t *testing.T) {
	due := &fakeDue{}

	processed, failed := cancellations(&fakeTenants{err: errors.New("identity-audit unavailable")}, due, &fakeCancellations{}, &bytes.Buffer{}).
		Run(context.Background(), 100)

	if processed != 0 || failed != 1 || len(due.asked) != 0 {
		t.Errorf("processed=%d failed=%d asked=%v, want 0, 1 and nothing asked", processed, failed, due.asked)
	}
}

// AC-WK-02.5: each tenant is asked for at most the batch (never above what the list gives), and a run whose time ended asks nothing more.
func TestA_a_run_reads_at_most_the_batch_per_tenant_and_stops_when_its_time_ends(t *testing.T) {
	due := &fakeDue{byTenant: map[string][]string{inactive: {"DEP-001", "DEP-002", "DEP-003"}, other: {"DEP-010"}}}
	c := &fakeCancellations{}

	cancellations(&fakeTenants{ids: []string{inactive}}, due, c, &bytes.Buffer{}).Run(context.Background(), 2)
	cancellations(&fakeTenants{ids: []string{inactive}}, due, &fakeCancellations{}, &bytes.Buffer{}).Run(context.Background(), 500)

	if !slices.Equal(due.limits, []int{2, usecase.MaxDueLimit}) || len(c.starts) != 2 {
		t.Errorf("limits=%v starts=%d, want limits [2 %d] and 2 starts", due.limits, len(c.starts), usecase.MaxDueLimit)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timed := &fakeDue{byTenant: due.byTenant}
	stopped := &fakeCancellations{onStart: cancel}

	processed, _ := cancellations(&fakeTenants{ids: []string{inactive, other}}, timed, stopped, &bytes.Buffer{}).Run(ctx, 100)

	if processed != 1 || len(stopped.starts) != 1 || !slices.Equal(timed.asked, []string{inactive}) {
		t.Errorf("processed=%d starts=%d asked=%v, want one start and the second tenant never asked", processed, len(stopped.starts), timed.asked)
	}
}

func TestA_the_cancellation_job_has_a_name(t *testing.T) {
	if got := cancellations(&fakeTenants{}, &fakeDue{}, &fakeCancellations{}, &bytes.Buffer{}).Name(); got != "inactive-tenant-cancellations" {
		t.Errorf("name = %q", got)
	}
}
