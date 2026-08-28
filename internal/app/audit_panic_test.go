package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestRunDurationAudit_PanicReportsDegraded verifies that a panicking audit
// worker is visible at the exported boundary. RunDurationAudit closes its
// correction channel on the way out of a panic exactly as it does after a clean
// run, so channel close alone cannot tell a caller that tracks went unaudited;
// the returned stage outcome is what makes the two distinguishable.
func TestRunDurationAudit_PanicReportsDegraded(t *testing.T) {
	restore := app.SetProbeFn(func(context.Context, string) (float64, error) {
		panic("ffprobe parser blew up")
	})
	t.Cleanup(restore)

	reader := &stubReader{tracks: []db.UnverifiedTrack{
		{Hash: "hash-1", Path: "/music/one.mp3", DurationMs: 120000},
		{Hash: "hash-2", Path: "/music/two.mp3", DurationMs: 180000},
	}}
	writer := newStubVerifier()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch, outcome := app.RunDurationAudit(ctx, reader, writer)

	drained := make(chan []library.DurationFix, 1)
	go func() {
		var fixes []library.DurationFix
		for f := range ch {
			fixes = append(fixes, f)
		}
		drained <- fixes
	}()

	select {
	case fixes := <-drained:
		if len(fixes) != 0 {
			t.Errorf("expected no corrections from a panicking audit, got %d", len(fixes))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("correction channel never closed after the audit workers panicked")
	}

	err := outcome.Err()
	if err == nil {
		t.Fatal("closed correction channel reported success after an audit worker panicked")
	}
	if !errors.Is(err, app.ErrDurationAuditIncomplete) {
		t.Errorf("outcome error = %v, want it to wrap ErrDurationAuditIncomplete", err)
	}
}

// TestRunDurationAudit_CleanRunReportsNoFailure is the negative control: the
// outcome must stay nil for an audit that really did finish, or the assertion
// above would hold for an implementation that reports failure unconditionally.
func TestRunDurationAudit_CleanRunReportsNoFailure(t *testing.T) {
	restore := app.SetProbeFn(func(context.Context, string) (float64, error) { return 120.0, nil })
	t.Cleanup(restore)

	reader := &stubReader{tracks: []db.UnverifiedTrack{
		{Hash: "hash-1", Path: "/music/one.mp3", DurationMs: 120000},
	}}
	writer := newStubVerifier()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch, outcome := app.RunDurationAudit(ctx, reader, writer)
	for range ch { //nolint:revive // draining to completion
	}
	if err := outcome.Err(); err != nil {
		t.Errorf("clean duration audit reported a failure: %v", err)
	}
}
