package app

import (
	"context"
	"log/slog"

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
	// (e.g. permission denied on a subdirectory). The channel is already closed
	// when LoadLibrary returns; drain it with a range loop or a Cmd.
	TuiScanErrCh <-chan library.ScanErrorMsg

	// TuiWatchCh delivers filesystem watch events to the TUI.
	TuiWatchCh <-chan library.WatchEvent

	// DeletedPaths lists files present in the DB but absent on disk.
	DeletedPaths []string

	// HashByPath maps file paths to BLAKE3 content hashes, used for
	// deletion-path lookups.
	HashByPath map[string]string

	// WatchDegraded is true when the filesystem watcher fell back to
	// polling mode (e.g. on unsupported filesystems).
	WatchDegraded bool
}

// LibraryWriter is the subset of db.Writer used by LoadLibrary for persisting
// scan and watch results.
type LibraryWriter interface {
	UpsertBatcher
	MissingMarker
}

// LoadLibrary reads cached tracks from the database, starts a diff scan for
// new/changed files, and launches the filesystem watcher. The returned
// LibraryResult contains everything needed to populate the TUI and keep it
// updated.
func LoadLibrary(ctx context.Context, database *db.DB, writer LibraryWriter,
	musicDir string, extractor library.MetadataExtractor,
	upsertBatchSize int) (LibraryResult, error) {

	// Load the cached library from the database for instant startup display.
	dbTracks, err := database.ReadTracks()
	if err != nil {
		slog.Warn("startup: read tracks from DB", "err", err)
	}

	// Build the library track slice and known-file map for the diff scan.
	libraryTracks := make([]library.Track, len(dbTracks))
	known := make(map[string]library.KnownFile, len(dbTracks))
	for i, row := range dbTracks {
		libraryTracks[i] = library.Track{
			Hash:      row.Hash,
			Path:      row.Path,
			Title:     row.Title,
			Artist:    row.Artist,
			Album:     row.Album,
			Duration:  float64(row.DurationMs) / 1000.0,
			ModTime:   row.ModTime,
			SizeBytes: row.SizeBytes,
		}
		known[row.Path] = library.KnownFile{
			ModTime:   row.ModTime,
			SizeBytes: row.SizeBytes,
		}
	}

	// Collect non-fatal walk errors into a buffered channel so the TUI can
	// surface them as status bar notifications. FilteredWalk runs synchronously
	// inside DiffScan, so all errors are enqueued before DiffScan returns and
	// we can close the channel immediately after.
	scanErrCh := make(chan library.ScanErrorMsg, 32)
	onWalkErr := func(err error) {
		select {
		case scanErrCh <- library.ScanErrorMsg{Err: err}:
		default: // drop if buffer is full
		}
	}

	// Diff scan: walk the filesystem and only process files that are new or
	// have changed since the last scan. Deletions are returned synchronously.
	rawScanCh, deletedPaths, walkErr := library.DiffScan(ctx, musicDir, extractor, known, onWalkErr)
	close(scanErrCh) // all walk errors have been enqueued; channel may be empty
	if walkErr != nil {
		slog.Warn("diff scan: walk failed, skipping deletions", "err", walkErr)
		deletedPaths = nil
		empty := make(chan library.Track)
		close(empty)
		rawScanCh = empty
	}

	// Bridge new/changed tracks to both the DB and the TUI scan channel.
	tuiScanCh := BridgeScanChannel(rawScanCh, writer, upsertBatchSize)

	// Build path→hash lookup from cached tracks. Used by the deletion-path
	// handler in both the pre-TUI eviction loop and the watcher bridge.
	hashByPath := make(map[string]string, len(libraryTracks))
	for _, t := range libraryTracks {
		hashByPath[t.Path] = t.Hash
	}

	rawWatchCh, watchDegraded, watchErr := library.Watch(ctx, musicDir, extractor, library.DefaultDebounce)
	if watchErr != nil {
		slog.Error("failed to start file watcher", "err", watchErr)
	}

	// Bridge watcher events to both the DB (for persistence) and the TUI
	// (for in-memory index updates).
	tuiWatchCh := BridgeWatchChannel(rawWatchCh, writer, writer, hashByPath, upsertBatchSize)

	return LibraryResult{
		Tracks:        libraryTracks,
		TuiScanCh:     tuiScanCh,
		TuiScanErrCh:  scanErrCh,
		TuiWatchCh:    tuiWatchCh,
		DeletedPaths:  deletedPaths,
		HashByPath:    hashByPath,
		WatchDegraded: watchDegraded,
	}, nil
}
