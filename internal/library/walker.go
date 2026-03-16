package library

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// audioExtensions is the set of supported audio file extensions (lowercase).
var audioExtensions = map[string]struct{}{
	".mp3":  {},
	".flac": {},
	".ogg":  {},
	".opus": {},
	".m4a":  {},
	".wav":  {},
	".wma":  {},
	".aac":  {},
	".aiff": {},
	".ape":  {},
	".wv":   {},
}

// Walk walks root recursively, sending audio file paths to the returned channel.
// It runs in a single goroutine and closes the channel when the walk completes.
// Hidden files and directories (names starting with '.') are skipped.
// Symbolic links are followed for files but not for directories.
func Walk(root string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				slog.Warn("walk error", "path", path, "err", err)
				return nil // skip unreadable entries
			}

			name := d.Name()

			// Skip hidden entries.
			if strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Resolve symlinks: follow for files, skip for directories.
			if d.Type()&fs.ModeSymlink != 0 {
				info, err := os.Stat(path)
				if err != nil {
					return nil
				}
				if info.IsDir() {
					return nil // do not follow directory symlinks
				}
				// It's a file symlink — check extension.
				if isAudioFile(info.Name()) {
					ch <- path
				}
				return nil
			}

			if d.IsDir() {
				return nil // descend normally
			}

			if isAudioFile(name) {
				ch <- path
			}
			return nil
		})
	}()
	return ch
}

// CountAudioFiles walks root using stat-only directory traversal and returns
// the number of audio files matching audioExtensions. It applies the same
// hidden-file, hidden-directory, and file-only-symlink rules as Walk so the
// count matches what the scanner pipeline will process. Intended to be called
// in a background goroutine to provide an early estimated total.
func CountAudioFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("count walk error", "path", path, "err", err)
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
				count++
			}
			return nil
		}
		if !d.IsDir() && isAudioFile(name) {
			count++
		}
		return nil
	})
	return count
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := audioExtensions[ext]
	return ok
}
