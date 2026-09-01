package app

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/safego"
)

// LibraryResult holds the outputs of LoadLibrary for use by the TUI and
// watcher systems.
type LibraryResult struct {
	// Tracks is the full set of library tracks built from the database.
	Tracks []library.Track

	// TuiScanCh delivers newly discovered/changed tracks to the TUI.
	TuiScanCh <-chan library.Track

	// TuiScanErrCh carries non-fatal errors from the initial diff scan (e.g.
	// permission denied on a subdirectory, or a hash stage that ended short
	// because a worker panicked) and, for the rest of the session, any later
	// failure of the background pipeline.
	//
	// It closes when the whole pipeline ends, not when the scan does. The
	// watcher forwarder outlives the initial walk by design, so a stream scoped
	// to the walk would be gone before the forwarder could report anything —
	// and since watcher availability is published once, up front, and the walk
	// closes cleanly, a late forwarder failure would leave every visible signal
	// saying the pipeline was healthy. Drain it with a range loop or a Cmd; it
	// stays open until StopAndWait, so expect to block rather than to see it
	// close after startup.
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
	// runLibraryPipeline defers close(done), so recovering in the spawn wrapper
	// still signals completion to LoadLibrary's waiters.
	pipelineArgs := libraryPipeline{
		musicDirs:      musicDirs,
		excludedPaths:  excludedPaths,
		extractor:      extractor,
		known:          known,
		rawScanStream:  rawScanStream,
		rawWatchStream: rawWatchStream,
		scanErrCh:      scanErrCh,
		watchStatusCh:  watchStatusCh,
	}
	safego.Go("app.runLibraryPipeline", func() {
		runLibraryPipeline(pipelineCtx, pipelineDone, pipelineArgs)
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

func isNilLibraryWriter(writer LibraryWriter) bool {
	if writer == nil {
		return true
	}
	v := reflect.ValueOf(writer)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
