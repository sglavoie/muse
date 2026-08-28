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

// TestRunLibraryPipeline_ForwarderPanicAfterScanCloseReportsDegraded is the
// late-session ordering, and the one TestRunLibraryPipeline_ChildPanicReportsDegraded
// structurally cannot reach: it holds the diff walk open until cancellation, so
// the scan-error stream is guaranteed to still be accepting sends when the
// forwarder panics.
//
// The real pipeline does the opposite. The initial walk finishes early and the
// forwarder runs for the whole session behind it, so the interesting panic is
// the one that fires *after* the walk is done. Watcher availability was already
// published as healthy and cannot be republished, the scan stream closed
// normally, and the error stream used to close with the walk — which left a
// dead forwarder looking exactly like a healthy one from the caller's side.
//
// The walk here therefore runs to completion and is observed to have completed
// before the forwarder is released to panic.
func TestRunLibraryPipeline_ForwarderPanicAfterScanCloseReportsDegraded(t *testing.T) {
	healthyWatch(t)

	// walkDone is closed by the diff walk on the way out, so the forwarder
	// cannot panic until the walk goroutine has actually returned and its
	// deferred closes have run. Without this handshake the test would be racing
	// the very ordering it exists to pin.
	walkDone := make(chan struct{})
	prevWalk := runDiffWalkFn
	t.Cleanup(func() { runDiffWalkFn = prevWalk })
	runDiffWalkFn = func(context.Context, *pipelineOutputs) { close(walkDone) }

	prevForward := forwardWatchEventsFn
	t.Cleanup(func() { forwardWatchEventsFn = prevForward })
	forwardWatchEventsFn = func(context.Context, <-chan library.WatchEvent, chan<- library.WatchEvent) {
		<-walkDone
		panic("watch forwarder blew up after the initial scan finished")
	}

	h := runPipelineForTest(t)

	// The whole point: the caller is still told, on a stream it is still
	// reading, that the pipeline died — even though the scan it was watching
	// completed cleanly and the watcher was last reported healthy.
	assertScanErrReported(t, h.scanErrCh)
	h.assertAllStreamsClosed(t)
}

// TestLoadLibraryScanErrCh_OutlivesTheInitialScan pins the lifetime the fix
// depends on at the public boundary: TuiScanErrCh must not close when the
// initial scan finishes, because the watcher forwarder still has the rest of
// the session in which to fail. A stream that closed with the scan would make
// every later report unreachable no matter how faithfully the pipeline recorded
// it.
func TestLoadLibraryScanErrCh_OutlivesTheInitialScan(t *testing.T) {
	healthyWatch(t)

	scanned := make(chan struct{})
	prevWalk := runDiffWalkFn
	t.Cleanup(func() { runDiffWalkFn = prevWalk })
	runDiffWalkFn = func(context.Context, *pipelineOutputs) { close(scanned) }

	h := pipelineHarness{
		scanStream:    make(chan library.Track, 1),
		watchStream:   make(chan library.WatchEvent, 1),
		scanErrCh:     make(chan library.ScanErrorMsg, 32),
		watchStatusCh: make(chan library.WatchStatusMsg, 1),
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go runLibraryPipeline(ctx, done, libraryPipeline{
		musicDirs:      []string{t.TempDir()},
		known:          map[string]library.KnownFile{},
		rawScanStream:  h.scanStream,
		rawWatchStream: h.watchStream,
		scanErrCh:      h.scanErrCh,
		watchStatusCh:  h.watchStatusCh,
	})

	select {
	case <-scanned:
	case <-time.After(10 * time.Second):
		t.Fatal("initial diff walk never ran")
	}
	// The scan stream closing is the observable proof that the walk goroutine
	// finished its defers; the error stream must not have gone with it.
	assertClosed(t, "scan", func() bool { _, ok := <-h.scanStream; return ok })
	select {
	case _, ok := <-h.scanErrCh:
		if !ok {
			t.Fatal("scan-error stream closed with the initial scan, leaving the watcher forwarder nowhere to report")
		}
	default: // open and empty, which is exactly right
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pipeline never signalled completion after cancellation")
	}
	assertClosed(t, "scan error", func() bool { _, ok := <-h.scanErrCh; return ok })
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
