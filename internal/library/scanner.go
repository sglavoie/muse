package library

import (
	"context"
	"os"
)

// Scan runs the three-stage scanner pipeline for root and returns a channel
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
func Scan(ctx context.Context, root string, extractor MetadataExtractor) <-chan Track {
	return ExtractBatches(ctx, extractor, HashWorkers(Walk(ctx, root)))
}

// KnownFile holds the cached stat info for a file that is already in the DB.
// It is used by FilteredWalk and DiffScan to skip files that have not changed.
type KnownFile struct {
	ModTime   int64 // Unix timestamp of last modification
	SizeBytes int64 // file size in bytes
}

// FilteredWalk walks root, applying the same hidden-file, audio-extension, and
// symlink rules as Walk, but skips files whose path exists in known with
// matching mtime and size. It returns:
//   - a slice of paths for files that are new or changed
//   - a slice of paths from known that were not seen during the walk (deleted files)
//   - an error if the walk itself fails (callers should skip the deletion step)
func FilteredWalk(ctx context.Context, root string, known map[string]KnownFile) ([]string, []string, error) {
	seen := make(map[string]bool, len(known))
	var changed []string

	if err := walkAudioPaths(ctx, root, func(path string) error {
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
	}); err != nil {
		return nil, nil, err
	}

	// Compute deletions: known paths not seen during the walk.
	var deleted []string
	for path := range known {
		if !seen[path] {
			deleted = append(deleted, path)
		}
	}

	return changed, deleted, nil
}

// pathsToChan streams paths into a buffered channel for HashWorkers consumption.
// The goroutine respects ctx cancellation so callers can abort without leaks.
func pathsToChan(ctx context.Context, paths []string) <-chan string {
	ch := make(chan string, 64)
	go func() {
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
// and an error if the walk fails. On walk failure, callers should skip
// the deletion step (deleted will be nil).
//
// On first run (empty known map) DiffScan degrades to a full scan.
func DiffScan(ctx context.Context, root string, extractor MetadataExtractor, known map[string]KnownFile) (<-chan Track, []string, error) {
	changed, deleted, err := FilteredWalk(ctx, root, known)
	if err != nil {
		return nil, nil, err
	}
	return ExtractBatches(ctx, extractor, HashWorkers(pathsToChan(ctx, changed))), deleted, nil
}
