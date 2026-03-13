package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce is the default debounce window for filesystem events.
const DefaultDebounce = 2500 * time.Millisecond

// Watch watches root for filesystem changes and triggers incremental rescans
// through the scanner pipeline. Events within debounceDelay are coalesced into
// a single rescan trigger. Rename/move events are treated as delete-plus-create:
// the Rename event for the old path is discarded and the subsequent Create event
// for the new path drives the rescan. Directory-level events trigger a recursive
// re-traversal of the affected subtree via Walk. Each affected file is extracted
// with a per-file ExifTool invocation (not batched).
//
// The returned channel emits Track values for rescanned files and is closed when
// ctx is cancelled.
func Watch(ctx context.Context, root string, extractor MetadataExtractor, debounceDelay time.Duration) (<-chan Track, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := addWatchesRecursive(w, root); err != nil {
		w.Close()
		return nil, err
	}

	out := make(chan Track, 64)
	go watchLoop(ctx, w, root, extractor, debounceDelay, out)
	return out, nil
}

// watchLoop is the main event loop for the watcher goroutine.
func watchLoop(ctx context.Context, w *fsnotify.Watcher, _ string, extractor MetadataExtractor, debounceDelay time.Duration, out chan<- Track) {
	defer close(out)
	defer w.Close()

	// pending maps path -> isDir for paths that need rescanning.
	pending := make(map[string]bool)

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
			handleFsEvent(event, pending, w)
			// Reset the debounce window.
			debounce = time.After(debounceDelay)

		case _, ok := <-w.Errors:
			if !ok {
				return
			}
			// Watcher errors are non-fatal; continue watching.

		case <-debounce:
			debounce = nil
			if len(pending) == 0 {
				continue
			}
			rescan(ctx, pending, extractor, out)
			pending = make(map[string]bool)
		}
	}
}

// handleFsEvent updates the pending rescan set based on a single fsnotify event.
// For Create events on directories, a new recursive watch is also registered.
func handleFsEvent(event fsnotify.Event, pending map[string]bool, w *fsnotify.Watcher) {
	path := event.Name

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
		} else if isAudioFile(filepath.Base(path)) {
			pending[path] = false
		}

	case event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod):
		if isAudioFile(filepath.Base(path)) {
			pending[path] = false
		}

	case event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove):
		// Treat as delete: remove from pending so a deleted file is not
		// rescanned. For renames, the new path arrives as a separate Create
		// event and will be added to pending at that point.
		delete(pending, path)
	}
}

// rescan processes all entries in pending: audio files are hashed and extracted
// individually (per-file ExifTool invocation); directory entries are traversed
// recursively via Walk and each audio file found is processed individually.
func rescan(ctx context.Context, pending map[string]bool, extractor MetadataExtractor, out chan<- Track) {
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

		// Per-file ExifTool invocation (not batch mode).
		tracks, err := extractor.Extract([]HashedFile{{Path: path, Hash: hash}})
		if err != nil {
			continue
		}
		for _, t := range tracks {
			select {
			case <-ctx.Done():
				return
			case out <- t:
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
			_ = w.Add(path)
		}
		return nil
	})
}
