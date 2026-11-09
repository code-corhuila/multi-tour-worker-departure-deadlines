package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"
	"github.com/code-corhuila/multi-tour-worker-runtime/logging"

	"github.com/code-corhuila/multi-tour-worker-departure-deadlines/internal/application/usecase"
)

// fakeDepartures answers each ask with the next scripted answer and remembers the limits it was asked with.
type fakeDepartures struct {
	answers []answer
	limits  []int
}

type answer struct {
	marked int
	err    error
}

func (f *fakeDepartures) MarkPendingRegularization(_ context.Context, limit int) (int, error) {
	f.limits = append(f.limits, limit)
	if len(f.limits) > len(f.answers) {
		return 0, nil
	}
	a := f.answers[len(f.limits)-1]
	return a.marked, a.err
}

func job(f *fakeDepartures, out *bytes.Buffer) *usecase.MarkPending {
	return usecase.NewMarkPending(f, logging.New(out))
}

// AC-WK-01.1: the batch of the run is the limit of the ask.
func TestA_asks_the_service_with_the_batch_as_the_limit(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{marked: 3}}}

	processed, failed := job(f, &bytes.Buffer{}).Run(context.Background(), 100)

	if processed != 3 || failed != 0 || len(f.limits) != 1 || f.limits[0] != 100 {
		t.Errorf("processed=%d failed=%d limits=%v, want 3, 0 and one ask of 100", processed, failed, f.limits)
	}
}

// AC-WK-01.2: a full batch means there may be more, so it asks again, never more than five times in a run.
func TestA_repeats_while_the_batch_is_full_and_stops_at_five(t *testing.T) {
	full := answer{marked: 10}
	f := &fakeDepartures{answers: []answer{full, full, full, full, full, full, full}}

	processed, failed := job(f, &bytes.Buffer{}).Run(context.Background(), 10)

	if len(f.limits) != usecase.MaxAsks || processed != 10*usecase.MaxAsks || failed != 0 {
		t.Errorf("asks=%d processed=%d failed=%d, want %d asks and %d marked", len(f.limits), processed, failed, usecase.MaxAsks, 10*usecase.MaxAsks)
	}
}

func TestA_stops_when_a_batch_is_not_full(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{marked: 10}, {marked: 4}, {marked: 10}}}

	processed, _ := job(f, &bytes.Buffer{}).Run(context.Background(), 10)

	if len(f.limits) != 2 || processed != 14 {
		t.Errorf("asks=%d processed=%d, want 2 asks and 14 marked", len(f.limits), processed)
	}
}

// AC-WK-01.4: nothing to mark is a quiet run.
func TestA_zero_marked_is_a_quiet_run(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{marked: 0}}}
	var out bytes.Buffer

	processed, failed := job(f, &out).Run(context.Background(), 100)

	if processed != 0 || failed != 0 || len(f.limits) != 1 {
		t.Errorf("processed=%d failed=%d asks=%d, want 0, 0 and one ask", processed, failed, len(f.limits))
	}
	if !strings.Contains(out.String(), `"marked":0`) {
		t.Errorf("the run should say none was marked: %s", out.String())
	}
}

// AC-WK-01.3: a failure ends the run, counted; what was marked before it stays counted; the next run asks again.
func TestA_a_failed_ask_ends_the_run_counted_and_the_next_run_asks_again(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{marked: 10}, {err: errors.New("service unavailable")}, {marked: 2}}}
	j := job(f, &bytes.Buffer{})

	processed, failed := j.Run(context.Background(), 10)
	if processed != 10 || failed != 1 || len(f.limits) != 2 {
		t.Errorf("processed=%d failed=%d asks=%d, want 10, 1 and 2 asks", processed, failed, len(f.limits))
	}
	processed, failed = j.Run(context.Background(), 10)
	if processed != 2 || failed != 0 {
		t.Errorf("next run: processed=%d failed=%d, want 2 and 0", processed, failed)
	}
}

func TestA_a_done_context_asks_nothing(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{marked: 1}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed, failed := job(f, &bytes.Buffer{}).Run(ctx, 10)

	if processed != 0 || failed != 0 || len(f.limits) != 0 {
		t.Errorf("processed=%d failed=%d asks=%d, want nothing", processed, failed, len(f.limits))
	}
}

// AC-WK-01.5: each line of the run carries the correlation id of the run.
func TestA_log_lines_carry_the_correlation_id(t *testing.T) {
	f := &fakeDepartures{answers: []answer{{err: errors.New("boom")}}}
	var out bytes.Buffer
	ctx := correlation.WithID(context.Background(), "run-42")

	job(f, &out).Run(ctx, 10)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("the run logged nothing")
	}
	for _, line := range lines {
		if !strings.Contains(line, `"correlationId":"run-42"`) {
			t.Errorf("line without the correlation id: %s", line)
		}
	}
}

func TestA_the_job_has_a_name(t *testing.T) {
	if got := job(&fakeDepartures{}, &bytes.Buffer{}).Name(); got != "departure-deadlines" {
		t.Errorf("name = %q", got)
	}
}
