package main

import (
	"context"
	"testing"
)

type fakeJob struct {
	ran              *[]string
	name             string
	processed, fails int
}

func (f fakeJob) Name() string { return f.name }

func (f fakeJob) Run(context.Context, int) (int, int) {
	*f.ran = append(*f.ran, f.name)
	return f.processed, f.fails
}

// Plan D-11: one run marks first and then starts the cancellations, with the same batch, and the runner sees the sum.
func TestInSequence_runs_each_job_in_order_and_sums_the_counts(t *testing.T) {
	var ran []string
	tick := inSequence{fakeJob{&ran, "mark", 3, 1}, fakeJob{&ran, "cancel", 2, 0}}

	processed, failed := tick.Run(context.Background(), 10)

	if processed != 5 || failed != 1 || len(ran) != 2 || ran[0] != "mark" || ran[1] != "cancel" {
		t.Errorf("processed=%d failed=%d ran=%v, want 5, 1 and mark then cancel", processed, failed, ran)
	}
}

// A run whose time is over starts nothing more: the next run asks again.
func TestInSequence_stops_when_the_run_is_over(t *testing.T) {
	var ran []string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inSequence{fakeJob{&ran, "mark", 0, 0}}.Run(ctx, 10)

	if len(ran) != 0 {
		t.Errorf("ran %v, want nothing", ran)
	}
}
