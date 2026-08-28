package app

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// healthyWatch installs a watchFn that reports an available watcher backed by a
// channel that stays open until the pipeline context is cancelled. The point is
// that the parent frame completes normally, so anything the test then observes
// comes from a child goroutine rather than from watcher registration.
func healthyWatch(t *testing.T) {
	t.Helper()
	prev := watchFn
	t.Cleanup(func() { watchFn = prev })
	watchFn = func(ctx context.Context, _ []string, _ []string, _ library.MetadataExtractor, _ time.Duration) (<-chan library.WatchEvent, bool, error) {
		ch := make(chan library.WatchEvent)
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, false, nil
	}
}

// TestRunLibraryPipeline_ChildPanicReportsDegraded verifies that a panic in
// either of runLibraryPipeline's child goroutines is reported to the caller
// rather than being laundered into a clean finish.
//
// Both children own stream closes, and both used to recover with a plain
// safego.Recover: the panic reached the log, the streams closed exactly as they
// would after a successful run, and the orchestrator's failure state stayed nil
// — so a watcher status that had already been reported healthy was the last
// thing the TUI ever heard. Recovery now routes through out.fail, which records
// the failure, publishes it on the scan-error stream the TUI renders, and
// cancels the pipeline so the sibling goroutine winds down.
func TestRunLibraryPipeline_ChildPanicReportsDegraded(t *testing.T) {
	t.Run("watch forwarder", func(t *testing.T) {
		healthyWatch(t)

		prevForward := forwardWatchEventsFn
		t.Cleanup(func() { forwardWatchEventsFn = prevForward })
		forwardWatchEventsFn = func(context.Context, <-chan library.WatchEvent, chan<- library.WatchEvent) {
			panic("watch forwarder blew up")
		}

		// Hold the diff walk open until the pipeline is cancelled, which is what
		// a real initial walk is doing while the forwarder runs. Without it the
		// walk of an empty directory could close the scan-error stream before
		// the forwarder's panic had anywhere to be reported, making the
		// assertion below race the walk.
		prevWalk := runDiffWalkFn
		t.Cleanup(func() { runDiffWalkFn = prevWalk })
		runDiffWalkFn = func(ctx context.Context, _ *pipelineOutputs) { <-ctx.Done() }

		h := runPipelineForTest(t)

		// The watcher registration succeeded, so the status channel already
		// carried Available:true. The failure therefore has to surface on the
		// scan-error stream.
		assertScanErrReported(t, h.scanErrCh)
		h.assertAllStreamsClosed(t)
	})

	t.Run("diff walk", func(t *testing.T) {
		healthyWatch(t)

		prevWalk := runDiffWalkFn
		t.Cleanup(func() { runDiffWalkFn = prevWalk })
		runDiffWalkFn = func(context.Context, *pipelineOutputs) { panic("diff walk blew up") }

		h := runPipelineForTest(t)

		assertScanErrReported(t, h.scanErrCh)
		h.assertAllStreamsClosed(t)
	})
}

// pipelineHarness holds the streams of one runLibraryPipeline invocation.
type pipelineHarness struct {
	scanStream    chan library.Track
	watchStream   chan library.WatchEvent
	scanErrCh     chan library.ScanErrorMsg
	watchStatusCh chan library.WatchStatusMsg
}

// runPipelineForTest starts runLibraryPipeline over a temp directory, drains the
// single watcher-availability message, and blocks until the pipeline signals
// completion. A pipeline that never completes is the other half of the defect
// under test, so the wait is bounded and fatal.
func runPipelineForTest(t *testing.T) pipelineHarness {
	t.Helper()
	h := pipelineHarness{
		scanStream:    make(chan library.Track, 1),
		watchStream:   make(chan library.WatchEvent, 1),
		scanErrCh:     make(chan library.ScanErrorMsg, 32),
		watchStatusCh: make(chan library.WatchStatusMsg, 1),
	}
	done := make(chan struct{})

	go runLibraryPipeline(t.Context(), done, libraryPipeline{
		musicDirs:      []string{t.TempDir()},
		known:          map[string]library.KnownFile{},
		rawScanStream:  h.scanStream,
		rawWatchStream: h.watchStream,
		scanErrCh:      h.scanErrCh,
		watchStatusCh:  h.watchStatusCh,
	})

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pipeline never signalled completion after a child goroutine panicked")
	}
	return h
}

// assertScanErrReported requires at least one scan error to be waiting on ch,
// which is the assertion that a closed stream cannot masquerade as a clean run.
func assertScanErrReported(t *testing.T, ch <-chan library.ScanErrorMsg) {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("scan error channel closed without reporting the child panic")
		}
		if msg.Err == nil {
			t.Error("scan error message carried no error")
		}
	default:
		t.Fatal("no scan error reported for the panicking child goroutine")
	}
}

func (h pipelineHarness) assertAllStreamsClosed(t *testing.T) {
	t.Helper()
	// Drain whatever else is queued, then require every stream to be closed so
	// no consumer is left hanging on a dead pipeline. A range that returns is
	// itself the closure assertion for the two streams that carry values here.
	for range h.scanErrCh { //nolint:revive // draining to close
	}
	for range h.watchStatusCh { //nolint:revive // draining to close
	}
	assertClosed(t, "scan", func() bool { _, ok := <-h.scanStream; return ok })
	assertClosed(t, "watch", func() bool { _, ok := <-h.watchStream; return ok })
}
