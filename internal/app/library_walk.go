package app

import (
	"context"
	"log/slog"

	"github.com/sglavoie/muse/internal/library"
)

// forwardWatchEventsFn and runDiffWalkFn indirect the two child goroutines of
// runLibraryPipeline so tests can inject the one failure a real filesystem
// cannot be made to produce on demand: a panic inside the child frame.
var (
	forwardWatchEventsFn = forwardWatchEvents
	runDiffWalkFn        = runDiffWalk
)

// forwardWatchEvents relays events from the raw watcher channel into the shared
// reconciliation stream, respecting cancellation on both the receive and the
// forward so a stopped pipeline never blocks.
func forwardWatchEvents(ctx context.Context, in <-chan library.WatchEvent, out chan<- library.WatchEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}

// runDiffWalk performs the filesystem diff walk, forwarding new/changed tracks
// to the scan bridge and initial deletions to the watcher reconciliation
// stream. Duplicate diff/watch observations are tolerated downstream as
// idempotent hash/path upserts and missing marks.
//
// The scan-error stream outlives this function — the pipeline closes it only
// once every goroutine has exited — so a hash-stage failure discovered after
// the walk still has somewhere to go.
func runDiffWalk(ctx context.Context, out *pipelineOutputs) {
	p := out.p
	// Collect non-fatal walk errors into the scan-error channel so the TUI can
	// surface them. FilteredWalk runs synchronously inside DiffScan, so all
	// errors are enqueued before DiffScan returns.
	onWalkErr := func(err error) { out.reportScanErr(err) }

	rawScanCh, deletedPaths, hashOutcome, walkErr := library.DiffScan(ctx, p.musicDirs, p.excludedPaths, p.extractor, p.known, onWalkErr)
	if walkErr != nil {
		slog.Warn("diff scan: walk failed, skipping deletions", "err", walkErr)
		deletedPaths = nil
		rawScanCh = closedTrackChan()
	}
	// A hash worker that panics closes the track stream just like a finished one
	// would, so report the truncation on the error stream the TUI renders rather
	// than letting the short scan pass for a complete one.
	defer func() {
		if err := hashOutcome.Err(); err != nil {
			slog.Error("diff scan: hash stage incomplete", "err", err)
			out.reportScanErr(err)
		}
	}()

	// Feed initial deletions into the same watcher reconciliation stream as
	// live deletions.
	for _, path := range deletedPaths {
		select {
		case p.rawWatchStream <- library.WatchEvent{Deleted: true, Path: path}:
		case <-ctx.Done():
			drainTracks(rawScanCh)
			return
		}
	}

	// Feed new/changed tracks through the scan bridge.
	for {
		select {
		case <-ctx.Done():
			drainTracks(rawScanCh)
			return
		case t, ok := <-rawScanCh:
			if !ok {
				return
			}
			select {
			case p.rawScanStream <- t:
			case <-ctx.Done():
				drainTracks(rawScanCh)
				return
			}
		}
	}
}

// drainTracks consumes any remaining values from ch so the DiffScan extraction
// goroutines (which self-terminate on ctx cancellation) can finish and close
// their output channel without leaking.
func drainTracks(ch <-chan library.Track) {
	for range ch {
	}
}

// closedTrackChan returns an already-closed track channel, used as a no-op scan
// source when the diff walk itself failed.
func closedTrackChan() <-chan library.Track {
	ch := make(chan library.Track)
	close(ch)
	return ch
}
