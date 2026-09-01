package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/safego"
)

// libraryPipeline bundles the inputs the asynchronous pipeline needs.
type libraryPipeline struct {
	musicDirs      []string
	excludedPaths  []string
	extractor      library.MetadataExtractor
	known          map[string]library.KnownFile
	rawScanStream  chan<- library.Track
	rawWatchStream chan<- library.WatchEvent
	scanErrCh      chan<- library.ScanErrorMsg
	watchStatusCh  chan<- library.WatchStatusMsg
}

// watchFn is library.Watch, indirected so tests can inject a watcher that fails
// in ways a real one cannot be made to on demand (notably by panicking).
var watchFn = library.Watch

// pipelineOutputs owns every outbound channel of runLibraryPipeline so the
// normal path and the panic-recovery path can close each one exactly once. Its
// once guards are what make the recovery path safe: without them a panic that
// fired after the diff walk had already closed its stream would close it twice.
type pipelineOutputs struct {
	p libraryPipeline

	statusOnce sync.Once
	scanOnce   sync.Once
	watchOnce  sync.Once

	// cancel stops the pipeline context. fail calls it so a panic in any frame
	// unblocks the sibling goroutines the deferred wg.Wait is about to wait on.
	cancel context.CancelFunc

	// errMu serialises every use of scanErrCh. Unlike the other streams it has
	// more than one writer — the diff-walk goroutine while it walks, and any
	// frame that recovers a panic — so the send and the close have to be under
	// the same lock or a late failure report would race the close. The close
	// itself belongs to finish, after every writer has been waited on.
	errMu     sync.Mutex
	errClosed bool

	// failureMu guards failure, which is written by runLibraryPipeline's own
	// recover handler and by the recover handlers of both child goroutines.
	failureMu sync.Mutex
	// failure holds the first panic recovered anywhere in the pipeline. The
	// degraded watcher status is published from finish rather than from the
	// recover handlers because the other goroutines are still running at
	// recover time.
	failure error
}

// fail records a recovered panic from any pipeline frame, publishes it on the
// scan-error stream, and cancels the pipeline so the remaining goroutines wind
// down instead of running on against a stream nobody will finish. The first
// failure wins: it is the one that actually broke the pipeline, and the
// cancellations it triggers can make the others report derived failures.
//
// Reporting happens here rather than in finish because finish runs after every
// pipeline goroutine has exited, and the degraded watcher status it can publish
// is a no-op once availability has already been reported healthy — which it
// always has been by the time a child goroutine can panic. The scan-error
// stream is the only surface that stays open for the whole pipeline, so it is
// the one a late failure has to reach, and it has to be reached here while the
// failing frame is still unwinding rather than held back until the close.
func (o *pipelineOutputs) fail(err error) {
	o.failureMu.Lock()
	if o.failure == nil {
		o.failure = err
	}
	o.failureMu.Unlock()
	o.reportScanErr(err)
	if o.cancel != nil {
		o.cancel()
	}
}

// failureErr returns the first recovered panic, or nil if the pipeline never
// failed.
func (o *pipelineOutputs) failureErr() error {
	o.failureMu.Lock()
	defer o.failureMu.Unlock()
	return o.failure
}

// reportStatus emits the single watcher-availability message and closes the
// channel. Later calls are no-ops, so whichever of the normal path and the
// failure path gets there first defines the reported status.
func (o *pipelineOutputs) reportStatus(msg library.WatchStatusMsg) {
	o.statusOnce.Do(func() {
		o.p.watchStatusCh <- msg
		close(o.p.watchStatusCh)
	})
}

func (o *pipelineOutputs) closeScan()  { o.scanOnce.Do(func() { close(o.p.rawScanStream) }) }
func (o *pipelineOutputs) closeWatch() { o.watchOnce.Do(func() { close(o.p.rawWatchStream) }) }

func (o *pipelineOutputs) closeErr() {
	o.errMu.Lock()
	defer o.errMu.Unlock()
	if o.errClosed {
		return
	}
	o.errClosed = true
	close(o.p.scanErrCh)
}

// reportScanErr publishes err on the scan-error stream the TUI already renders,
// dropping it when the stream has closed or its buffer is full.
func (o *pipelineOutputs) reportScanErr(err error) {
	o.errMu.Lock()
	defer o.errMu.Unlock()
	if o.errClosed {
		return
	}
	select {
	case o.p.scanErrCh <- library.ScanErrorMsg{Err: err}:
	default: // drop if buffer is full
	}
}

// finish publishes the degraded watcher status for a recovered panic, then
// closes every stream. It runs after the pipeline's goroutines have been waited
// on, which is what makes it the right owner of the error stream's close: every
// frame that could still report a failure has already returned. The scan error
// itself was already sent by fail.
func (o *pipelineOutputs) finish() {
	if failure := o.failureErr(); failure != nil {
		// A no-op when the watcher status was already reported successfully;
		// the scan error fail sent still carries the failure in that case.
		o.reportStatus(library.WatchStatusMsg{Available: false, Err: failure})
	}
	o.closeScan()
	o.closeErr()
	o.closeWatch()
}

// runLibraryPipeline registers the filesystem watcher, starts draining live
// watcher events, then runs the diff walk. Watcher registration precedes the
// walk so no on-disk change that lands between the two is missed. done is
// closed when every goroutine this function spawns has exited.
//
// A panic in this function is recovered rather than fatal, but it must not look
// like a clean finish: close(done) alone would report success to LoadLibrary's
// waiters while leaving the TUI's streams open forever. The deferred chain
// therefore cancels the pipeline, waits for the spawned goroutines, publishes
// the failure as a scan error and a degraded watcher status, and only then
// closes the streams and done.
func runLibraryPipeline(ctx context.Context, done chan<- struct{}, p libraryPipeline) {
	ctx, cancel := context.WithCancel(ctx)
	out := &pipelineOutputs{p: p, cancel: cancel}
	var wg sync.WaitGroup

	// Deferred calls run last-registered-first: recover, wait, finish,
	// close(done), cancel. Cancellation is last so the normal path is not cut
	// short by it; the recovery handler cancels early instead (out.fail), which
	// is what lets the wg.Wait that follows it return on a failed pipeline.
	defer cancel()
	defer close(done)
	defer out.finish()
	defer wg.Wait()
	defer safego.RecoverTo("app.runLibraryPipeline", out.fail)

	// 1. Register the filesystem watcher before the diff walk so no change that
	//    lands between the walk and watch registration is missed.
	rawWatchCh, degraded, watchErr := watchFn(ctx, p.musicDirs, p.excludedPaths, p.extractor, library.DefaultDebounce)
	if watchErr != nil {
		slog.Error("failed to start file watcher", "err", watchErr)
		degraded = true
	}
	// Report watcher availability asynchronously, then close the channel.
	out.reportStatus(library.WatchStatusMsg{Available: !degraded, Err: watchErr})

	// 2. Drain live watcher events into the reconciliation stream immediately,
	//    concurrently with the diff walk below.
	wg.Add(1)
	go func() {
		// wg.Done stays deferred so a recovered panic still lets the wg.Wait
		// above reach out.closeWatch and drain the watch bridge. RecoverTo
		// rather than Recover: a panic here stops live event forwarding for the
		// rest of the session, so it has to reach out.failure and cancel the
		// pipeline instead of quietly closing an already-healthy stream.
		defer wg.Done()
		defer safego.RecoverTo("app.forwardWatchEvents", out.fail)
		forwardWatchEventsFn(ctx, rawWatchCh, p.rawWatchStream)
	}()

	// 3. Run the diff walk: feed new/changed tracks through the scan bridge and
	//    initial deletions into the same watcher reconciliation stream as live
	//    deletions.
	wg.Add(1)
	go func() {
		// Registration order is inverted here relative to the repo's usual
		// "recover first" idiom, because defers unwind last-registered-first
		// and the recovery has to run *before* the close: closing the scan
		// stream after a panic is indistinguishable from a finished walk unless
		// out.fail has already published the failure.
		//
		// Only the scan stream is closed here. The error stream is not
		// walk-scoped — the watcher forwarder outlives this goroutine and must
		// still have somewhere to report a panic — so finish closes it once
		// every writer has been waited on.
		defer wg.Done()
		defer out.closeScan()
		defer safego.RecoverTo("app.runDiffWalk", out.fail)
		runDiffWalkFn(ctx, out)
	}()
}
