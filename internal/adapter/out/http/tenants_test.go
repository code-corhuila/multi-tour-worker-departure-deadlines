package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	deadlineshttp "github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/adapter/out/http"
)

const tenantTwo = "22222222-2222-4222-8222-222222222222"

// pagedService answers the n-th call with the n-th page (the last one again after that) and remembers each path, query and credential.
type pagedService struct {
	mu      sync.Mutex
	pages   []string
	paths   []string
	queries []string
	auth    []string
}

func newPaged(t *testing.T, status int, pages ...string) (*deadlineshttp.Tenants, *pagedService) {
	t.Helper()
	p := &pagedService{pages: pages}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.paths, p.queries = append(p.paths, r.URL.Path), append(p.queries, r.URL.RawQuery)
		p.auth = append(p.auth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":"FORBIDDEN","message":"m","traceId":"t"}`))
			return
		}
		_, _ = w.Write([]byte(p.pages[min(len(p.queries), len(p.pages))-1]))
	}))
	t.Cleanup(srv.Close)
	return deadlineshttp.NewTenants(newClient(srv.URL)), p
}

func page(totalPages string, items ...string) string {
	return `{"data":[` + strings.Join(items, ",") + `],"meta":{"page":1,"limit":100,"total":1,"totalPages":` + totalPages + `}}`
}

func inactive(id string) string { return `{"tenantId":"` + id + `","status":"INACTIVE"}` }

// I·RN-WK-020: the inactive tenants come from the internal route of identity-audit, page by page, with the token of the service.
func TestTenants_reads_every_page_of_the_inactive_tenants(t *testing.T) {
	a, seen := newPaged(t, http.StatusOK, page("2", inactive(tenantOne)), page("2", inactive(tenantTwo)))

	got, err := a.ListInactive(ctx())

	if err != nil || len(got) != 2 || got[0] != tenantOne || got[1] != tenantTwo {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(seen.queries) != 2 || seen.paths[0] != "/api/v1/identity-audit/internal/tenants" ||
		seen.queries[0] != "status=INACTIVE&page=1&limit=100" || seen.queries[1] != "status=INACTIVE&page=2&limit=100" {
		t.Errorf("requests = %v %v", seen.paths, seen.queries)
	}
	if seen.auth[0] != "Bearer "+token {
		t.Errorf("auth = %q, want the service token", seen.auth[0])
	}
}

func TestTenants_no_inactive_tenant_is_an_empty_list(t *testing.T) {
	a, seen := newPaged(t, http.StatusOK, page("0"))

	got, err := a.ListInactive(ctx())

	if err != nil || len(got) != 0 || len(seen.queries) != 1 {
		t.Fatalf("got %v, %v after %d calls", got, err, len(seen.queries))
	}
}

// An id that is not a UUID, or a tenant that is not inactive, is an error and never a partial list: the saga would cancel what came back.
func TestTenants_a_refusal_a_bad_id_or_an_active_tenant_is_an_error(t *testing.T) {
	refused, _ := newPaged(t, http.StatusForbidden)
	if got, err := refused.ListInactive(ctx()); err == nil || got != nil || strings.Contains(err.Error(), token) {
		t.Errorf("a 403 gave %v, %v", got, err)
	}
	for _, item := range []string{inactive("../x"), `{"tenantId":"` + tenantTwo + `","status":"ACTIVE"}`, `{"status":"INACTIVE"}`} {
		a, _ := newPaged(t, http.StatusOK, page("1", inactive(tenantOne), item))
		if got, err := a.ListInactive(ctx()); err == nil || got != nil {
			t.Errorf("item %s gave %v, %v", item, got, err)
		}
	}
}

func TestTenants_a_service_that_never_stops_paging_is_an_error(t *testing.T) {
	a, seen := newPaged(t, http.StatusOK, page("100000", inactive(tenantOne)))

	got, err := a.ListInactive(ctx())

	if err == nil || got != nil || len(seen.queries) != 1000 {
		t.Errorf("got %v, %v after %d calls; want an error after 1000", got, err, len(seen.queries))
	}
}
