package library

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/safego"
)

// Scan runs the three-stage scanner pipeline for roots and returns a channel
// that receives Track values as batches complete:
//
//  1. A single walker goroutine emits audio file paths into a buffered channel.
//  2. runtime.NumCPU() hash worker goroutines consume paths concurrently and
//     emit HashedFile values into a second buffered channel.
//  3. A single ExifTool batch goroutine accumulates HashedFile values and
//     invokes extractor.Extract on each ~500-file batch, emitting Track values.
//
// The returned channel is closed once the walk has finished and all hash and
// ExifTool goroutines have drained, signalling scan-complete. Callers drain
// the channel with a range loop; channel close is the completion signal.
//
// Channel close alone does not mean the scan was complete, so Scan also returns
// the hash stage's StageOutcome. Once the channel has been drained, a non-nil
// Err means a hash worker panicked and the emitted tracks are a subset of what
// is on disk — a caller that derives deletions from the scan result must not do
// so in that case.
func Scan(ctx context.Context, roots []string, excludedPaths []string, extractor MetadataExtractor) (<-chan Track, *StageOutcome) {
	hashed, outcome := HashWorkers(Walk(ctx, roots, excludedPaths))
	return ExtractBatches(ctx, extractor, hashed), outcome
}

// KnownFile holds the cached stat info for a file that is already in the DB.
// It is used by FilteredWalk and DiffScan to skip files that have not changed.
type KnownFile struct {
	ModTime   int64 // Unix timestamp of last modification
	SizeBytes int64 // file size in bytes
}

// FilteredWalk walks each root in roots, applying the same hidden-file,
// audio-extension, and symlink rules as Walk, but skips files whose path
// exists in known with matching mtime and size. It returns:
//   - a slice of paths for files that are new or changed
//   - a slice of paths from known that were not seen during the walk (deleted files)
//   - an error if any walk itself fails (callers should skip the deletion step)
//
// A root that cannot be opened at all (missing directory, permission denied) is
// logged and skipped rather than failing the whole walk, so a user whose
// configured music directory does not exist still gets a working, empty
// library. Deletions are suppressed for that root only: known paths beneath a
// skipped root were not observed, so reporting them as deleted would mark that
// part of the cached library missing. Known paths beneath the roots that were
// walked successfully are still reconciled in the same scan, so one unmounted
// music directory does not freeze deletion reconciliation everywhere else.
//
// onErr, when non-nil, is called for each non-fatal walk error so callers can
// surface them to the user (e.g. permission denied on a subdirectory).
func FilteredWalk(ctx context.Context, roots []string, excludedPaths []string, known map[string]KnownFile, onErr func(error)) ([]string, []string, error) {
	seen := make(map[string]bool, len(known))
	var changed []string
	var skippedRoots []string

	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		if err := walkAudioPaths(ctx, root, excludedPaths, func(path string) error {
			if seen[path] {
				return nil
			}
			seen[path] = true
			info, err := os.Stat(path)
			if err != nil {
				changed = append(changed, path)
				return nil
			}
			kf, exists := known[path]
			if !exists || info.ModTime().Unix() != kf.ModTime || info.Size() != kf.SizeBytes {
				changed = append(changed, path)
			}
			return nil
		}, onErr); err != nil {
			if errors.Is(err, errRootSkipped) {
				slog.Warn("filtered walk: skipping unreadable root", "root", root, "err", err)
				skippedRoots = append(skippedRoots, root)
				continue
			}
			return nil, nil, err
		}
	}

	// A cancelled context truncates the walk at an arbitrary point in an
	// arbitrary root, so nothing about the unseen known paths can be trusted.
	// Return nil deletions so the caller skips the deletion step entirely.
	if ctx.Err() != nil {
		return changed, nil, nil
	}

	// Compute deletions: known paths not seen during the walk.
	// Skip paths excluded by the current configuration so that a newly
	// excluded directory is not reported as "deleted" and marked missing.
	// Skip paths beneath a root that could not be opened: those files were
	// never observed, which is not the same as having been deleted. Deletions
	// under the roots that were walked successfully are still reported, so an
	// unmounted root only freezes reconciliation for its own subtree.
	var deleted []string
	for path := range known {
		if seen[path] || ShouldExclude(path, excludedPaths) {
			continue
		}
		if underAnyRoot(path, skippedRoots) {
			continue
		}
		deleted = append(deleted, path)
	}

	return changed, deleted, nil
}

// underAnyRoot reports whether path lies inside (or is) any of roots.
func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if underRoot(path, root) {
			return true
		}
	}
	return false
}

// underRoot reports whether path lies inside root or is root itself. Both are
// cleaned first so a configured root with a trailing separator or a "." segment
// cannot change the answer; known paths come from a walk of the same roots, so
// the textual comparison matches without resolving symlinks.
func underRoot(path, root string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if path == root {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	return strings.HasPrefix(path, root)
}

// pathsToChan streams paths into a buffered channel for HashWorkers consumption.
// The goroutine respects ctx cancellation so callers can abort without leaks.
func pathsToChan(ctx context.Context, paths []string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer safego.Recover("library.pathsToChan")
		defer close(ch)
		for _, p := range paths {
			select {
			case ch <- p:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// DiffScan is like Scan but skips files whose path, mtime, and size match an
// entry in known. It returns a channel of new or changed Track values, a
// slice of paths that were in known but no longer exist on disk (deletions),
// the hash stage's StageOutcome, and an error if the walk fails. On walk
// failure, callers should skip the deletion step (deleted will be nil).
//
// As in Scan, the StageOutcome is what separates a drained track channel from a
// complete one: check its Err after the channel closes to learn whether a hash
// worker panicked and truncated the batch. It is nil only when the walk failed
// and no hash stage was started.
//
// onErr, when non-nil, is called for each non-fatal walk error encountered
// during the directory traversal (e.g. permission denied on a subdirectory).
//
// On first run (empty known map) DiffScan degrades to a full scan.
func DiffScan(ctx context.Context, roots []string, excludedPaths []string, extractor MetadataExtractor, known map[string]KnownFile, onErr func(error)) (<-chan Track, []string, *StageOutcome, error) {
	changed, deleted, err := FilteredWalk(ctx, roots, excludedPaths, known, onErr)
	if err != nil {
		return nil, nil, nil, err
	}
	hashed, outcome := HashWorkers(pathsToChan(ctx, changed))
	return ExtractBatches(ctx, extractor, hashed), deleted, outcome, nil
}
