package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/config"
)

func env(pairs ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(k string) string { return m[k] }
}

var required = []string{
	"SERVICE_TOKEN", "secret-token-value",
	"OPERATIONS_COST_BASE_URL", "http://operations-cost-api:8084/",
}

func with(extra ...string) []string { return append(append([]string{}, required...), extra...) }

// AC-WK-01.7: the declared defaults, and the shared runtime variables travel with them.
func TestLoadDeadlines_applies_the_declared_defaults(t *testing.T) {
	got, err := config.LoadDeadlines(env(required...))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Interval != 15*time.Minute || got.HealthAddr != ":8080" || got.Runtime.BatchSize != 100 {
		t.Errorf("defaults = %+v", got)
	}
	if got.OperationsCostURL != "http://operations-cost-api:8084" {
		t.Errorf("url = %q, the trailing slash should go", got.OperationsCostURL)
	}
}

func TestLoadDeadlines_reads_its_own_variables(t *testing.T) {
	got, err := config.LoadDeadlines(env(with("DEADLINES_INTERVAL", "1m", "HEALTH_ADDR", ":9090")...))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Interval != time.Minute || got.HealthAddr != ":9090" {
		t.Errorf("got %+v", got)
	}
}

// AC-WK-01.7: a missing or malformed variable stops the worker with its name, never its value.
func TestLoadDeadlines_names_the_bad_variable_and_never_prints_a_value(t *testing.T) {
	cases := map[string][]string{
		"SERVICE_TOKEN":            {"OPERATIONS_COST_BASE_URL", "http://operations-cost-api:8084"},
		"OPERATIONS_COST_BASE_URL": {"SERVICE_TOKEN", "secret-token-value"},
		"DEADLINES_INTERVAL":       with("DEADLINES_INTERVAL", "soon"),
	}
	for name, pairs := range cases {
		_, err := config.LoadDeadlines(env(pairs...))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error = %v, want one that names it", name, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret-token-value") {
			t.Errorf("%s: the error prints the token: %v", name, err)
		}
	}
}

func TestLoadDeadlines_refuses_an_interval_that_is_not_positive(t *testing.T) {
	for _, bad := range []string{"0", "0s", "-5m"} {
		if _, err := config.LoadDeadlines(env(with("DEADLINES_INTERVAL", bad)...)); err == nil {
			t.Errorf("DEADLINES_INTERVAL=%q accepted, want an error", bad)
		}
	}
}

// The service refuses a limit above 500 (api spec 005): the worker stops at start instead of failing in every run, and says which variable.
func TestLoadDeadlines_refuses_a_batch_size_above_what_the_service_accepts(t *testing.T) {
	if _, err := config.LoadDeadlines(env(with("BATCH_SIZE", "500")...)); err != nil {
		t.Errorf("BATCH_SIZE=500 refused: %v", err)
	}
	for _, bad := range []string{"501", "1000"} {
		_, err := config.LoadDeadlines(env(with("BATCH_SIZE", bad)...))
		if err == nil || !strings.Contains(err.Error(), "BATCH_SIZE") {
			t.Errorf("BATCH_SIZE=%s: error = %v, want one that names the variable", bad, err)
		}
	}
}

// A base URL holds only scheme, host and port: a credential, a path, a query or a fragment would reach every request.
func TestLoadDeadlines_refuses_a_base_url_with_more_than_scheme_host_and_port(t *testing.T) {
	for _, bad := range []string{"operations-cost-api:8084", "ftp://x:1", "http://user:pass@x:1", "http://x:1/api", "http://x:1?q=1", "http://x:1#f", ""} {
		_, err := config.LoadDeadlines(env("SERVICE_TOKEN", "t", "OPERATIONS_COST_BASE_URL", bad))
		if err == nil || !strings.Contains(err.Error(), "OPERATIONS_COST_BASE_URL") {
			t.Errorf("url %q: error = %v, want one that names the variable", bad, err)
		}
	}
}
