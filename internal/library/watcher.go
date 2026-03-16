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
var watcherFactory = func() (*fsnotify.Watcher, error) {
	return fsnotify.NewWatcher()
}

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

// Watch watches root for filesystem changes and triggers incremental rescans
// through the scanner pipeline. Events within debounceDelay are coalesced into
// a single rescan trigger. Rename/move events are treated as delete-plus-create:
// the Rename event for the old path emits a deletion WatchEvent and the
// subsequent Create event for the new path drives the rescan. Directory-level
// events trigger a recursive re-traversal of the affected subtree via Walk.
// Each affected file is extracted with a per-file ExifTool invocation (not batched).
//
// The returned channel emits WatchEvent values and is closed when ctx is
// cancelled. If fsnotify setup fails (e.g. fd exhaustion on macOS), Watch
// returns a pre-closed channel and a nil error: the watcher is inactive and the
// caller should rely on manual rescan as the sole rescan mechanism.
func Watch(ctx context.Context, root string, extractor MetadataExtractor, debounceDelay time.Duration) (<-chan WatchEvent, error) {
	raiseFDLimit()

	w, err := watcherFactory()
	if err != nil {
		slog.Warn("watcher: failed to create fsnotify watcher; falling back to manual rescan", "err", err)
		return closedWatchEventChan(), nil
	}

	if err := addWatchesRecursive(w, root); err != nil {
		w.Close()
		slog.Warn("watcher: failed to register directory watches; falling back to manual rescan", "err", err)
		return closedWatchEventChan(), nil
	}

	out := make(chan WatchEvent, 64)
	go watchLoop(ctx, w, root, extractor, debounceDelay, out)
	return out, nil
}

// closedWatchEventChan returns a channel that is already closed, signalling to
// callers that no events will ever be emitted (degraded / no-watcher mode).
func closedWatchEventChan() <-chan WatchEvent {
	ch := make(chan WatchEvent)
	close(ch)
	return ch
}

// watchLoop is the main event loop for the watcher goroutine.
func watchLoop(ctx context.Context, w *fsnotify.Watcher, _ string, extractor MetadataExtractor, debounceDelay time.Duration, out chan<- WatchEvent) {
	defer close(out)
	defer w.Close()

	// pending maps path -> isDir for paths that need rescanning.
	pending := make(map[string]bool)

	// deleted holds audio file paths deleted/renamed-away during the current
	// debounce window. These are emitted as deletion WatchEvents before rescanning.
	deleted := make(map[string]struct{})

	// debounce is reassigned on each new event; when it fires the pending set
	// is flushed. Using time.After means each event creates a new timer and the
	// previous one's channel is simply abandoned (fires into the void, then GC'd).
	var debounce <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-w.Events:
			if !ok {
				return
			}
			slog.Debug("watcher: fsnotify event", "op", event.Op.String(), "path", event.Name)
			handleFsEvent(event, pending, deleted, w)
			// Reset the debounce window.
			debounce = time.After(debounceDelay)

		case _, ok := <-w.Errors:
			if !ok {
				return
			}
			// Watcher errors are non-fatal; continue watching.

		case <-debounce:
			debounce = nil
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
			rescan(ctx, pending, extractor, out)
			pending = make(map[string]bool)
			deleted = make(map[string]struct{})
		}
	}
}

// handleFsEvent updates the pending rescan set and deleted set based on a
// single fsnotify event. For Create events on directories, a new recursive
// watch is also registered.
func handleFsEvent(event fsnotify.Event, pending map[string]bool, deleted map[string]struct{}, w *fsnotify.Watcher) {
	path := event.Name
	base := filepath.Base(path)

	// Skip hidden files and directories (matching Walk's behaviour).
	if strings.HasPrefix(base, ".") {
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
			_ = addWatchesRecursive(w, path)
			pending[path] = true
		} else if isAudioFile(base) {
			pending[path] = false
		}

	case event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod):
		if isAudioFile(base) {
			pending[path] = false
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

// rescan processes all entries in pending: audio files are hashed and extracted
// individually (per-file ExifTool invocation); directory entries are traversed
// recursively via Walk and each audio file found is processed individually.
func rescan(ctx context.Context, pending map[string]bool, extractor MetadataExtractor, out chan<- WatchEvent) {
	// Build a deduplicated set of audio file paths to process.
	files := make(map[string]struct{})
	for path, isDir := range pending {
		if isDir {
			// Recursively collect audio files in the affected subtree.
			for p := range Walk(path) {
				files[p] = struct{}{}
			}
		} else {
			files[path] = struct{}{}
		}
	}

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

		hf := HashedFile{Path: path, Hash: hash}
		if info, statErr := os.Stat(path); statErr == nil {
			hf.ModTime = info.ModTime().Unix()
			hf.SizeBytes = info.Size()
		}

		// Per-file ExifTool invocation (not batch mode).
		tracks, err := extractor.Extract([]HashedFile{hf})
		if err != nil {
			// Apply the same filename fallback used by extractWithBinaryRetry so
			// no track is ever silently dropped from the library.
			base := filepath.Base(path)
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			slog.Warn("watcher: metadata extraction failed, using filename fallback",
				"path", path, "error", err)
			tracks = []Track{{
				Path:      path,
				Hash:      hash,
				Artist:    "Unknown Artist",
				Title:     stem,
				ModTime:   hf.ModTime,
				SizeBytes: hf.SizeBytes,
			}}
		}
		for _, t := range tracks {
			select {
			case <-ctx.Done():
				return
			case out <- WatchEvent{Track: t}:
			}
		}
	}
}

// addWatchesRecursive registers an fsnotify watch on root and every
// subdirectory it contains. Errors on individual directories are ignored so
// that a single unreadable directory does not abort the entire watch setup.
func addWatchesRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})
}
