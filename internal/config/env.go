package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	runtimeconfig "github.com/code-corhuila/multi-tour-worker-runtime/config"
)

const (
	defaultInterval = 15 * time.Minute // ADR-004 D10, HU-WRK-003 AC1
	// maxBatchSize is the most the service marks in one ask (its limit is 1 to 500, api spec 005); a bigger batch would be refused
	// with a 400 in every run, so the worker stops at start instead.
	maxBatchSize = 500
)

// Deadlines is everything the worker reads from the environment: the shared runtime variables plus the interval, the health
// address and the internal base URL of the only service it calls.
type Deadlines struct {
	Runtime           runtimeconfig.Config
	Interval          time.Duration
	HealthAddr        string
	OperationsCostURL string
}

// LoadDeadlines reads the variables through getenv. Anything missing or malformed stops the worker at start; an error names
// the variable and never prints a value.
func LoadDeadlines(getenv func(string) string) (Deadlines, error) {
	shared, err := runtimeconfig.Load(getenv)
	if err != nil {
		return Deadlines{}, err
	}
	if shared.BatchSize > maxBatchSize {
		return Deadlines{}, fmt.Errorf("BATCH_SIZE must be from 1 to %d, the most that operations-cost marks in one ask", maxBatchSize)
	}
	d := Deadlines{Runtime: shared, Interval: defaultInterval, HealthAddr: ":8080"}
	if s := getenv("DEADLINES_INTERVAL"); s != "" {
		if d.Interval, err = time.ParseDuration(s); err != nil || d.Interval <= 0 {
			return Deadlines{}, fmt.Errorf("DEADLINES_INTERVAL must be a positive duration")
		}
	}
	if s := getenv("HEALTH_ADDR"); s != "" {
		d.HealthAddr = s
	}
	if d.OperationsCostURL, err = baseURL(getenv, "OPERATIONS_COST_BASE_URL"); err != nil {
		return Deadlines{}, err
	}
	return d, nil
}

func baseURL(getenv func(string) string, name string) (string, error) {
	raw := strings.TrimRight(getenv(name), "/")
	u, err := url.Parse(raw)
	// Only scheme://host[:port]: credentials, a query, a fragment or a path would reach every request.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", fmt.Errorf("%s must be an http(s) url with only scheme, host and port", name)
	}
	return raw, nil
}
