package app

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// LibraryResult holds the outputs of LoadLibrary for use by the TUI and
// watcher systems.
type LibraryResult struct {
	// Tracks is the full set of library tracks built from the database.
	Tracks []library.Track

	// TuiScanCh delivers newly discovered/changed tracks to the TUI.
	TuiScanCh <-chan library.Track

	// TuiScanErrCh carries non-fatal walk errors from the initial diff scan
	// (e.g. permission denied on a subdirectory). The channel is closed only
	// after the background walk completes; drain it with a range loop or a Cmd.
	TuiScanErrCh <-chan library.ScanErrorMsg

	// TuiWatchCh delivers filesystem watch events to the TUI. Initial deletions
	// (files present in the DB but absent on disk) arrive here as deletion
	// events, interleaved with live watcher deletions and changes.
	TuiWatchCh <-chan library.WatchEvent

	// TuiWatchStatusCh reports the filesystem watcher's availability once its
	// asynchronous setup completes. It emits exactly one WatchStatusMsg and is
	// then closed.
	TuiWatchStatusCh <-chan library.WatchStatusMsg

	// FreshLibrary is true when no tracks existed in the database before
	// the scan. Used to trigger first-launch UX (sort by recently added).
	FreshLibrary bool

	// lifecycle holds the cancellation and completion handles for the
	// asynchronous pipeline. It lives behind a pointer so LibraryResult stays
	// copyable (no embedded mutex) while every copy shares one stop latch.
	lifecycle *libraryLifecycle
}

// libraryLifecycle owns the pipeline cancel func and the completion signals of
// every background bridge and relay goroutine started by LoadLibrary.
type libraryLifecycle struct {
	cancel context.CancelFunc
	once   sync.Once
	dones  []<-chan struct{}
}

// StopAndWait cancels the asynchronous library pipeline and blocks until every
// bridge and relay goroutine has exited, giving pending database batches their
// final bounded flush attempt (each bridge drains once on cancellation). It is
// idempotent: repeated or concurrent calls collapse to a single stop, so it is
// safe to call from shutdown paths that may fire more than once.
func (r LibraryResult) StopAndWait() {
	if r.lifecycle == nil {
		return
	}
	r.lifecycle.once.Do(func() {
		if r.lifecycle.cancel != nil {
			r.lifecycle.cancel()
		}
		for _, done := range r.lifecycle.dones {
			<-done
		}
	})
}

// LibraryWriter is the subset of db.Writer used by LoadLibrary for persisting
// scan and watch results.
type LibraryWriter interface {
	UpsertBatcher
	MissingMarker
}

// LoadLibrary reads cached tracks from the database and returns them
// immediately for instant startup display. Only the database read and cached
// index construction are synchronous; the filesystem diff walk and watcher
// registration run asynchronously under a child pipeline context.
//
// The returned LibraryResult carries live output channels the TUI drains as the
// background pipeline produces results: new/changed tracks on TuiScanCh, watch
// and initial-deletion events on TuiWatchCh, non-fatal walk errors on
// TuiScanErrCh, and one watcher-availability message on TuiWatchStatusCh.
// StopAndWait cancels the pipeline and waits for a clean shutdown.
func LoadLibrary(ctx context.Context, database *db.DB, writer LibraryWriter,
	musicDirs []string, excludedPaths []string, extractor library.MetadataExtractor,
	upsertBatchSize int) (LibraryResult, error) {
	if isNilLibraryWriter(writer) {
		return LibraryResult{}, fmt.Errorf("startup: library writer is required")
	}

	// Synchronous cached-first read: load the cached library from the database
	// for instant startup display. This is the only blocking work.
	dbTracks, err := database.ReadTracks()
	if err != nil {
		return LibraryResult{}, fmt.Errorf("startup: read tracks from DB: %w", err)
	}

	// Build the library track slice, the known-file map for the diff walk, and
	// the path→hash lookup for deletion handling. All synchronous.
	libraryTracks := make([]library.Track, len(dbTracks))
	known := make(map[string]library.KnownFile, len(dbTracks))
	hashByPath := make(map[string]string, len(dbTracks))
	for i, row := range dbTracks {
		libraryTracks[i] = library.Track{
			Hash:         row.Hash,
			Path:         row.Path,
			Title:        row.Title,
			Artist:       row.Artist,
			Album:        row.Album,
			Genre:        row.Genre,
			Duration:     float64(row.DurationMs) / 1000.0,
			CreatedAt:    row.CreatedAt,
			ModTime:      row.ModTime,
			SizeBytes:    row.SizeBytes,
			PlayCount:    int(row.PlayCount),
			LastPlayedAt: row.LastPlayedAt,
			TrackNumber:  row.TrackNumber,
			DiscNumber:   row.DiscNumber,
		}
		known[row.Path] = library.KnownFile{
			ModTime:   row.ModTime,
			SizeBytes: row.SizeBytes,
		}
		hashByPath[row.Path] = row.Hash
	}

	// Child pipeline context: StopAndWait cancels it to stop all background
	// filesystem work and give pending DB batches a final flush.
	pipelineCtx, cancel := context.WithCancel(ctx)

	// Owned intermediary streams. The bridges start draining these immediately
	// so live events flow the instant they arrive; the background pipeline
	// feeds them once traversal produces results.
	rawScanStream := make(chan library.Track, scanChannelBuffer)
	rawWatchStream := make(chan library.WatchEvent, watchChannelBuffer)
	scanErrCh := make(chan library.ScanErrorMsg, 32)
	watchStatusCh := make(chan library.WatchStatusMsg, 1)

	// Shared path/hash state consulted by watcher deletion handling. Seeding it
	// from the cached DB rows and letting both bridges mutate it is what lets a
	// track first discovered by the startup scan be resolved to its hash (and
	// marked missing) when a later live deletion arrives. Its internal mutex
	// keeps concurrent scan and watch observations race-free.
	registry := newPathHashRegistry(hashByPath)

	tuiScanCh, scanDone := bridgeScanChannel(pipelineCtx, rawScanStream, writer, registry, upsertBatchSize)
	tuiWatchCh, watchDone := bridgeWatchChannel(pipelineCtx, rawWatchStream, writer, writer, registry, upsertBatchSize)

	pipelineDone := make(chan struct{})
	go runLibraryPipeline(pipelineCtx, pipelineDone, libraryPipeline{
		musicDirs:      musicDirs,
		excludedPaths:  excludedPaths,
		extractor:      extractor,
		known:          known,
		rawScanStream:  rawScanStream,
		rawWatchStream: rawWatchStream,
		scanErrCh:      scanErrCh,
		watchStatusCh:  watchStatusCh,
	})

	return LibraryResult{
		Tracks:           libraryTracks,
		TuiScanCh:        tuiScanCh,
		TuiScanErrCh:     scanErrCh,
		TuiWatchCh:       tuiWatchCh,
		TuiWatchStatusCh: watchStatusCh,
		FreshLibrary:     len(dbTracks) == 0,
		lifecycle: &libraryLifecycle{
			cancel: cancel,
			// Waiting on both bridge dones and the orchestrator done covers all
			// five background goroutines (two bridges, the watcher forwarder,
			// the diff walk, and the orchestrator).
			dones: []<-chan struct{}{scanDone, watchDone, pipelineDone},
		},
	}, nil
}

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

// runLibraryPipeline registers the filesystem watcher, starts draining live
// watcher events, then runs the diff walk. Watcher registration precedes the
// walk so no on-disk change that lands between the two is missed. done is
// closed when every goroutine this function spawns has exited.
func runLibraryPipeline(ctx context.Context, done chan<- struct{}, p libraryPipeline) {
	defer close(done)

	// 1. Register the filesystem watcher before the diff walk so no change that
	//    lands between the walk and watch registration is missed.
	rawWatchCh, degraded, watchErr := library.Watch(ctx, p.musicDirs, p.excludedPaths, p.extractor, library.DefaultDebounce)
	if watchErr != nil {
		slog.Error("failed to start file watcher", "err", watchErr)
		degraded = true
	}
	// Report watcher availability asynchronously, then close the channel.
	p.watchStatusCh <- library.WatchStatusMsg{Available: !degraded, Err: watchErr}
	close(p.watchStatusCh)

	var wg sync.WaitGroup

	// 2. Drain live watcher events into the reconciliation stream immediately,
	//    concurrently with the diff walk below.
	wg.Add(1)
	go func() {
		defer wg.Done()
		forwardWatchEvents(ctx, rawWatchCh, p.rawWatchStream)
	}()

	// 3. Run the diff walk: feed new/changed tracks through the scan bridge and
	//    initial deletions into the same watcher reconciliation stream as live
	//    deletions.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(p.rawScanStream)
		runDiffWalk(ctx, p)
	}()

	// The watch reconciliation stream stays open until both the live watcher
	// forwarder and the initial-deletion injector have finished, then closes so
	// the watch bridge can drain its final batch and exit.
	wg.Wait()
	close(p.rawWatchStream)
}

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
func runDiffWalk(ctx context.Context, p libraryPipeline) {
	// Collect non-fatal walk errors into the scan-error channel so the TUI can
	// surface them. FilteredWalk runs synchronously inside DiffScan, so all
	// errors are enqueued before DiffScan returns.
	onWalkErr := func(err error) {
		select {
		case p.scanErrCh <- library.ScanErrorMsg{Err: err}:
		default: // drop if buffer is full
		}
	}

	rawScanCh, deletedPaths, walkErr := library.DiffScan(ctx, p.musicDirs, p.excludedPaths, p.extractor, p.known, onWalkErr)
	// The walk has completed; close the error channel only now so consumers see
	// a genuinely asynchronous, walk-scoped error stream.
	close(p.scanErrCh)
	if walkErr != nil {
		slog.Warn("diff scan: walk failed, skipping deletions", "err", walkErr)
		deletedPaths = nil
		rawScanCh = closedTrackChan()
	}

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

func isNilLibraryWriter(writer LibraryWriter) bool {
	if writer == nil {
		return true
	}
	v := reflect.ValueOf(writer)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
