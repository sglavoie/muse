package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/audio"
)

// walkAudioPaths walks root applying the shared rules: skip hidden
// files/dirs, follow file symlinks only, and filter by audio extension.
// For each qualifying audio file path, it calls visit. If visit returns
// filepath.SkipDir or a non-nil error, the walk stops accordingly.
// If ctx is cancelled, the walk stops early. onErr, when non-nil, is called
// for each non-fatal walk error (e.g. permission denied on a subdirectory)
// in addition to the slog.Warn that is always emitted.
func walkAudioPaths(ctx context.Context, root string, visit func(path string) error, onErr func(error)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			if onErr != nil {
				onErr(err)
			}
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
				slog.Debug("skipping broken symlink", "path", path, "err", err)
				return nil
			}
			if info.IsDir() {
				slog.Warn("skipping symlinked directory (not followed)", "path", path)
				return nil
			}
			if isAudioFile(info.Name()) {
				return visit(path)
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if isAudioFile(name) {
			return visit(path)
		}
		return nil
	})
}

// Walk walks each root in roots recursively, sending audio file paths to the
// returned channel. Roots are walked sequentially in order; all paths are sent
// to the same channel which is closed when all walks complete or ctx is
// cancelled. Hidden files and directories (names starting with '.') are
// skipped. Symbolic links are followed for files but not for directories.
func Walk(ctx context.Context, roots []string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		for _, root := range roots {
			if ctx.Err() != nil {
				return
			}
			if err := walkAudioPaths(ctx, root, func(path string) error {
				select {
				case ch <- path:
					return nil
				case <-ctx.Done():
					return filepath.SkipAll
				}
			}, nil); err != nil && ctx.Err() == nil {
				slog.Warn("walk: root traversal error", "root", root, "err", err)
			}
		}
	}()
	return ch
}

// CountAudioFiles walks each root in roots and returns the total number of
// audio files matching audio.Extensions. It applies the same hidden-file,
// hidden-directory, and file-only-symlink rules as Walk so the count matches
// what the scanner pipeline will process. Intended to be called in a
// background goroutine to provide an early estimated total.
func CountAudioFiles(ctx context.Context, roots []string) int {
	count := 0
	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		if err := walkAudioPaths(ctx, root, func(_ string) error {
			count++
			return nil
		}, nil); err != nil && ctx.Err() == nil {
			slog.Warn("count audio files: walk error", "root", root, "err", err)
		}
	}
	return count
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := audio.Extensions[ext]
	return ok
}
