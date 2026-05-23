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
func walkAudioPaths(ctx context.Context, root string, excludedPaths []string, visit func(path string) error, onErr func(error)) error {
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
		// Don't apply the hidden-file rule to the root entry itself: a user
		// may legitimately specify a root whose basename starts with '.'
		// (e.g. "/Users/me/.musiclib"). Without this guard the entire walk
		// would abort on the first call.
		if strings.HasPrefix(name, ".") && path != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ShouldExclude(path, excludedPaths) {
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
			// Use the symlink's own name for the audio-extension check.
			// info.Name() can vary by platform for stat-resolved targets
			// (some return the target's base, some the path's base); the
			// original dirent name is the stable, intended check.
			if isAudioFile(name) {
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
// Directories whose name matches any entry in excludedPaths are skipped.
func Walk(ctx context.Context, roots []string, excludedPaths []string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		emitted := make(map[string]struct{})
		for _, root := range roots {
			if ctx.Err() != nil {
				return
			}
			if err := walkAudioPaths(ctx, root, excludedPaths, func(path string) error {
				if _, ok := emitted[path]; ok {
					return nil
				}
				emitted[path] = struct{}{}
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
// audio files matching audio.IsSupportedExtension. It applies the same hidden-file,
// hidden-directory, and file-only-symlink rules as Walk so the count matches
// what the scanner pipeline will process. Intended to be called in a
// background goroutine to provide an early estimated total.
func CountAudioFiles(ctx context.Context, roots []string, excludedPaths []string) int {
	count := 0
	seen := make(map[string]struct{})
	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		if err := walkAudioPaths(ctx, root, excludedPaths, func(path string) error {
			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}
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
	return audio.IsSupportedExtension(ext)
}
