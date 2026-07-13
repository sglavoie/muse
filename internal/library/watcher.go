package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watcherFactory creates a new fsnotify.Watcher. It is a variable so tests can
// substitute a factory that returns an error to exercise graceful degradation.
var watcherFactory = fsnotify.NewWatcher

// RegistrationStepHook is a test-only seam (used by both library and app tests).
// When non-nil it is invoked once per directory during recursive watch
// registration, immediately after the per-directory context check, so a test
// can gate registration mid-walk and prove cancellation aborts promptly without
// finishing the tree walk. Production leaves it nil.
var RegistrationStepHook func(path string)

// DefaultDebounce is the default debounce window for filesystem events.
const DefaultDebounce = 2500 * time.Millisecond

// WatchEvent is emitted on the channel returned by Watch. It carries either a
// newly-extracted Track (Deleted==false) or a deletion notification
// (Deleted==true, Path set, Track zero).
type WatchEvent struct {
	Track   Track
	Deleted bool
	Path    string // populated when Deleted==true; the deleted/renamed-away path
}

// Watch watches each root in roots for filesystem changes and triggers
// incremental rescans through the scanner pipeline. Events within debounceDelay
// are coalesced into a single rescan trigger. Rename/move events are treated as
// delete-plus-create: the Rename event for the old path emits a deletion
// WatchEvent and the subsequent Create event for the new path drives the
// rescan. Directory-level events trigger a recursive re-traversal of the
// affected subtree via Walk. Each affected file is extracted with a per-file
// ExifTool invocation (not batched).
//
// The returned channel emits WatchEvent values and is closed when ctx is
// cancelled. If fsnotify setup fails (e.g. fd exhaustion on macOS), Watch
// returns a pre-closed channel and a nil error: the watcher is inactive and the
// caller should rely on manual rescan as the sole rescan mechanism.
func Watch(ctx context.Context, roots []string, excludedPaths []string, extractor MetadataExtractor, debounceDelay time.Duration) (<-chan WatchEvent, bool, error) {
	raiseFDLimit()

	w, err := watcherFactory()
	if err != nil {
		slog.Warn("watcher: failed to create fsnotify watcher; falling back to manual rescan", "err", err)
		return closedWatchEventChan(), true, nil
	}

	degraded := false
	for _, root := range roots {
		if err := addWatchesRecursive(ctx, w, root, excludedPaths); err != nil {
			if ctx.Err() != nil {
				// Registration was cancelled mid-walk (e.g. shutdown racing
				// startup). Close the partially initialized watcher and fall
				// back to manual rescan without finishing the remaining tree
				// walk, so callers waiting on this return (and thus StopAndWait)
				// are not blocked until the whole traversal completes.
				_ = w.Close()
				return closedWatchEventChan(), true, nil
			}
			slog.Warn("watcher: failed to register directory watches for root", "root", root, "err", err)
			degraded = true
		}
	}
	if degraded && len(roots) > 0 {
		_ = w.Close()
		slog.Warn("watcher: falling back to manual rescan due to watch registration failures")
		return closedWatchEventChan(), true, nil
	}

	out := make(chan WatchEvent, 64)
	go watchLoop(ctx, w, excludedPaths, extractor, debounceDelay, out)
	return out, false, nil
}

// closedWatchEventChan returns a channel that is already closed, signalling to
// callers that no events will ever be emitted (degraded / no-watcher mode).
func closedWatchEventChan() <-chan WatchEvent {
	ch := make(chan WatchEvent)
	close(ch)
	return ch
}

// watchLoop is the main event loop for the watcher goroutine.
func watchLoop(ctx context.Context, w *fsnotify.Watcher, excludedPaths []string, extractor MetadataExtractor, debounceDelay time.Duration, out chan<- WatchEvent) {
	defer close(out)
	defer w.Close()

	// pending maps path -> isDir for paths that need rescanning.
	pending := make(map[string]bool)

	// deleted holds audio file paths deleted/renamed-away during the current
	// debounce window. These are emitted as deletion WatchEvents before rescanning.
	deleted := make(map[string]struct{})

	// debounceTimer coalesces rapid filesystem events into a single rescan.
	// It starts inactive and is reset on each new event.
	debounceTimer := time.NewTimer(debounceDelay)
	debounceTimer.Stop() // start inactive

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-w.Events:
			if !ok {
				return
			}
			slog.Debug("watcher: fsnotify event", "op", event.Op.String(), "path", event.Name)
			handleFsEvent(ctx, event, excludedPaths, pending, deleted, w)
			// Reset the debounce window.
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(debounceDelay)

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Warn("watcher: fsnotify error", "err", err)

		case <-debounceTimer.C:
			if len(pending) == 0 && len(deleted) == 0 {
				continue
			}
			// Emit deletion notifications first so the TUI can evict stale entries
			// before the rescanned tracks for new/modified files arrive.
			for path := range deleted {
				select {
				case <-ctx.Done():
					return
				case out <- WatchEvent{Deleted: true, Path: path}:
				}
			}
			rescan(ctx, pending, excludedPaths, extractor, out)
			pending = make(map[string]bool)
			deleted = make(map[string]struct{})
		}
	}
}

// handleFsEvent updates the pending rescan set and deleted set based on a
// single fsnotify event. For Create events on directories, a new recursive
// watch is also registered.
func handleFsEvent(ctx context.Context, event fsnotify.Event, excludedPaths []string, pending map[string]bool, deleted map[string]struct{}, w *fsnotify.Watcher) {
	path := event.Name
	base := filepath.Base(path)

	// Skip hidden files and directories (matching Walk's behaviour).
	if strings.HasPrefix(base, ".") {
		return
	}

	// Skip events for excluded directories.
	if ShouldExclude(path, excludedPaths) {
		return
	}

	switch {
	case event.Has(fsnotify.Create):
		info, err := os.Stat(path)
		if err != nil {
			// File may have been removed before we could stat it; skip.
			return
		}
		if info.IsDir() {
			// Register watches on the new directory tree and schedule a
			// re-traversal of the entire subtree.
			if err := addWatchesRecursive(ctx, w, path, excludedPaths); err != nil {
				slog.Debug("watcher: add recursive watches", "path", path, "err", err)
			}
			pending[path] = true
			// A create supersedes any earlier delete recorded in the same
			// debounce window (e.g. rename old→new then new→old, or rapid
			// editor save-temp dance). Clearing keeps the TUI from briefly
			// evicting a still-present entry before the rescan re-adds it.
			delete(deleted, path)
		} else if isAudioFile(base) {
			pending[path] = false
			delete(deleted, path)
		}

	case event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod):
		if isAudioFile(base) {
			if _, err := os.Stat(path); err != nil {
				// Some platforms can emit trailing metadata events after a
				// remove/rename. Do not let those erase the pending deletion.
				return
			}
			pending[path] = false
			// Write/Chmod on an existing path supersedes any earlier delete.
			delete(deleted, path)
		}

	case event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove):
		// Treat as delete: remove from pending so a deleted file is not
		// rescanned. For renames, the new path arrives as a separate Create
		// event and will be added to pending at that point.
		delete(pending, path)
		// Record audio file deletions so the TUI can evict the stale index entry.
		if isAudioFile(base) {
			deleted[path] = struct{}{}
		}
	}
}

// rescan processes all entries in pending: audio files are hashed and then
// extracted in a single batched ExifTool invocation. Directory entries are
// traversed recursively via Walk. On batch failure, extractWithBinaryRetry
// subdivides the batch and falls back to filename metadata for corrupt files.
func rescan(ctx context.Context, pending map[string]bool, excludedPaths []string, extractor MetadataExtractor, out chan<- WatchEvent) {
	// Build a deduplicated set of audio file paths to process.
	files := make(map[string]struct{})
	for path, isDir := range pending {
		if isDir {
			// Recursively collect audio files in the affected subtree.
			for p := range Walk(ctx, []string{path}, excludedPaths) {
				files[p] = struct{}{}
			}
		} else {
			files[path] = struct{}{}
		}
	}

	// Hash and stat every file, collecting HashedFile values for batch extraction.
	var hashed []HashedFile
	for path := range files {
		select {
		case <-ctx.Done():
			return
		default:
		}

		hash, err := HashFile(path)
		if err != nil {
			continue
		}

		// If Stat fails (file removed between hashing and statting), skip
		// emitting rather than passing zero ModTime/SizeBytes downstream.
		// Zero values would cause DiffScan to flag the path as changed on
		// every subsequent rescan. Mirrors hashWorkersN behaviour.
		info, statErr := os.Stat(path)
		if statErr != nil {
			slog.Warn("watcher rescan: stat after hash failed, skipping", "path", path, "err", statErr)
			continue
		}
		hashed = append(hashed, HashedFile{
			Path:      path,
			Hash:      hash,
			ModTime:   info.ModTime().Unix(),
			SizeBytes: info.Size(),
		})
	}

	if len(hashed) == 0 {
		return
	}

	// Single batched ExifTool invocation; extractWithBinaryRetry handles
	// subdivision and filename fallback on failure, mirroring the main scanner.
	tracks := extractWithBinaryRetry(ctx, extractor, hashed)
	for _, t := range tracks {
		select {
		case <-ctx.Done():
			return
		case out <- WatchEvent{Track: t}:
		}
	}
}

// addWatchesRecursive registers an fsnotify watch on root and every
// subdirectory it contains. Errors on individual directories are ignored so
// that a single unreadable directory does not abort the entire watch setup.
// Excluded directories are skipped and not registered for watching.
//
// The walk observes ctx: if it is cancelled mid-traversal, WalkDir stops
// promptly and returns ctx.Err() so shutdown never has to wait for a large tree
// to finish registering. The caller (Watch) closes the partially initialized
// watcher on that error.
func addWatchesRecursive(ctx context.Context, w *fsnotify.Watcher, root string, excludedPaths []string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if RegistrationStepHook != nil {
			RegistrationStepHook(path)
		}
		if err != nil {
			if path == root {
				return err
			}
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			if path != root && ShouldExclude(path, excludedPaths) {
				return filepath.SkipDir
			}
			if err := w.Add(path); err != nil {
				if path == root {
					return err
				}
				slog.Debug("watcher: add watch", "path", path, "err", err)
			}
		}
		return nil
	})
}
