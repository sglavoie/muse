package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/audio"
	"github.com/sglavoie/muse/internal/safego"
)

// errRootSkipped is returned by walkAudioPaths when the root itself could not
// be opened (missing directory, permission denied). It is a skip signal, not a
// failure: callers log it, yield an empty result for that root and carry on
// with the remaining ones, so a first-run user whose configured music
// directory does not exist still reaches a usable UI.
//
// FilteredWalk additionally suppresses deletion reporting for the paths
// beneath a skipped root. "No files found under an unreadable root" must never
// be mistaken for "every file under it was deleted", which would mark that
// music directory's tracks missing the first time it is unmounted or renamed.
// The suppression is scoped to that root's subtree, so the remaining roots are
// still reconciled normally.
var errRootSkipped = errors.New("root skipped")

// walkAudioPaths walks root applying the shared rules: skip hidden
// files/dirs, follow file symlinks only, and filter by audio extension.
// For each qualifying audio file path, it calls visit. If visit returns
// filepath.SkipDir or a non-nil error, the walk stops accordingly.
// If ctx is cancelled, the walk stops early. onErr, when non-nil, is called
// for each non-fatal walk error (e.g. permission denied on a subdirectory)
// in addition to the slog.Warn that is always emitted.
//
// A failure on the root entry itself is reported as an error wrapping
// errRootSkipped; every other entry-level failure is non-fatal and only
// reaches onErr.
func walkAudioPaths(ctx context.Context, root string, excludedPaths []string, visit func(path string) error, onErr func(error)) error {
	roots := []string{root}
	return walkDirFollowingRoot(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			if onErr != nil {
				onErr(err)
			}
			if path == root {
				return fmt.Errorf("%w: %s: %w", errRootSkipped, root, err)
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
		if ShouldExcludeUnder(path, roots, excludedPaths) {
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

// walkDirFollowingRoot is filepath.WalkDir, except that a root which is itself
// a symlink to a directory is followed. WalkDir Lstats the root, so a music
// directory configured as a symlink would otherwise be visited as a single
// non-directory entry and yield nothing, which FilteredWalk would then read as
// every known track having been deleted. The resolved directory is traversed,
// but fn sees each path under the configured root, so stored track paths,
// exclusion patterns and watch registrations keep the spelling the user chose.
// Only the root is followed; symlinked subdirectories are still not.
func walkDirFollowingRoot(root string, fn fs.WalkDirFunc) error {
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return filepath.WalkDir(root, fn)
	}
	target, err := filepath.EvalSymlinks(root)
	if err != nil {
		// A dangling root symlink is an unreadable root, reported the way
		// WalkDir reports a root it cannot Lstat.
		if err := fn(root, nil, err); err != nil && !errors.Is(err, filepath.SkipDir) && !errors.Is(err, filepath.SkipAll) {
			return err
		}
		return nil
	}
	if targetInfo, statErr := os.Stat(target); statErr != nil || !targetInfo.IsDir() {
		return filepath.WalkDir(root, fn)
	}
	return filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if path == target {
			path = root
		} else if rel, relErr := filepath.Rel(target, path); relErr == nil {
			path = filepath.Join(root, rel)
		}
		return fn(path, d, err)
	})
}

// Walk walks each root in roots recursively, sending audio file paths to the
// returned channel. Roots are walked sequentially in order; all paths are sent
// to the same channel which is closed when all walks complete or ctx is
// cancelled. Hidden files and directories (names starting with '.') are
// skipped. Symbolic links are followed for files but not for directories.
// Directories whose name matches any entry in excludedPaths are skipped.
// A root that cannot be opened is logged and skipped; the remaining roots are
// still walked.
func Walk(ctx context.Context, roots []string, excludedPaths []string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		// close(ch) stays deferred so a recovered panic still terminates the
		// downstream range over ch instead of hanging it.
		defer safego.Recover("library.Walk")
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
