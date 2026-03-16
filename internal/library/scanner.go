package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
func Scan(root string, extractor MetadataExtractor) <-chan Track {
	return ExtractBatches(extractor, HashWorkers(Walk(root)))
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
//   - a channel of paths for files that are new or changed (closed when walk is done)
//   - a slice of paths from known that were not seen during the walk (deleted files)
//
// The walk runs synchronously before returning so that deleted paths are
// computed deterministically. Changed paths are then streamed via the channel.
func FilteredWalk(root string, known map[string]KnownFile) (<-chan string, []string) {
	seen := make(map[string]bool, len(known))
	var changed []string

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		name := d.Name()

		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			info, err := os.Stat(path)
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if isAudioFile(info.Name()) {
				seen[path] = true
				kf, exists := known[path]
				if !exists || info.ModTime().Unix() != kf.ModTime || info.Size() != kf.SizeBytes {
					changed = append(changed, path)
				}
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if isAudioFile(name) {
			seen[path] = true
			info, err := d.Info()
			if err != nil {
				changed = append(changed, path)
				return nil
			}
			kf, exists := known[path]
			if !exists || info.ModTime().Unix() != kf.ModTime || info.Size() != kf.SizeBytes {
				changed = append(changed, path)
			}
		}
		return nil
	})

	// Compute deletions: known paths not seen during the walk.
	var deleted []string
	for path := range known {
		if !seen[path] {
			deleted = append(deleted, path)
		}
	}

	// Stream changed paths asynchronously so callers can pipeline into HashWorkers.
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		for _, p := range changed {
			ch <- p
		}
	}()

	return ch, deleted
}

// DiffScan is like Scan but skips files whose path, mtime, and size match an
// entry in known. It returns a channel of new or changed Track values and a
// slice of paths that were in known but no longer exist on disk (deletions).
//
// On first run (empty known map) DiffScan degrades to a full scan.
func DiffScan(root string, extractor MetadataExtractor, known map[string]KnownFile) (<-chan Track, []string) {
	changedCh, deleted := FilteredWalk(root, known)
	return ExtractBatches(extractor, HashWorkers(changedCh)), deleted
}
