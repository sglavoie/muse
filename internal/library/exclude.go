package library

import (
	"path/filepath"
	"runtime"
	"strings"
)

// foldPathPrefixes makes directory-prefix patterns ignore case. macOS volumes
// are case-insensitive by default, so "/users/me/music/ai" names the same
// directory as "/Users/me/Music/ai"; Linux filesystems are case-sensitive.
// It is a variable so tests can exercise both behaviours on any platform.
var foldPathPrefixes = runtime.GOOS == "darwin"

// ShouldExclude reports whether path should be excluded based on the
// excludedPaths patterns, checking bare-name patterns against every component
// of path. It is ShouldExcludeUnder with no music roots; callers that know the
// roots should use that instead so the roots' own ancestry is never matched.
func ShouldExclude(path string, excludedPaths []string) bool {
	return ShouldExcludeUnder(path, nil, excludedPaths)
}

// ShouldExcludeUnder reports whether path should be excluded based on the
// excludedPaths patterns. A pattern containing a path separator is treated as
// a directory prefix: it matches when path starts with pattern+"/" or equals
// the pattern exactly, ignoring case on macOS. A pattern without a separator
// is matched exactly (case-sensitive) against the components of path below
// the music root that contains it (both directory segments and the final
// element), so excluding "Music" cannot hide a library rooted at ~/Music.
// When several roots contain path (nested roots), it is excluded only if it is
// excluded below each of them, since any one walk would otherwise reach it.
// A path under none of roots has all its components checked. Returns false
// immediately when excludedPaths is empty.
func ShouldExcludeUnder(path string, roots []string, excludedPaths []string) bool {
	if len(excludedPaths) == 0 {
		return false
	}

	sep := string(filepath.Separator)

	for _, pattern := range excludedPaths {
		if !strings.Contains(pattern, sep) {
			continue
		}
		clean := strings.TrimRight(pattern, sep)
		// A pattern that is only separator characters (e.g. "/" or "////")
		// degenerates to "" after trimming and would otherwise prefix-match
		// every absolute path, excluding the entire library. Skip it.
		if clean == "" {
			continue
		}
		if hasPathPrefix(path, clean) {
			return true
		}
	}

	contained := false
	for _, root := range roots {
		if !underRoot(path, root) {
			continue
		}
		contained = true
		rel := strings.TrimPrefix(filepath.Clean(path), filepath.Clean(root))
		if !componentExcluded(rel, excludedPaths) {
			return false
		}
	}
	return contained || componentExcluded(path, excludedPaths)
}

// hasPathPrefix reports whether path is dir or lies beneath it.
func hasPathPrefix(path, dir string) bool {
	if len(path) < len(dir) {
		return false
	}
	head, rest := path[:len(dir)], path[len(dir):]
	if foldPathPrefixes {
		if !strings.EqualFold(head, dir) {
			return false
		}
	} else if head != dir {
		return false
	}
	return rest == "" || rest[0] == filepath.Separator
}

// componentExcluded reports whether any component of path equals a bare-name
// pattern.
func componentExcluded(path string, excludedPaths []string) bool {
	sep := string(filepath.Separator)
	for part := range strings.SplitSeq(path, sep) {
		if part == "" || part == "." {
			continue
		}
		for _, pattern := range excludedPaths {
			if !strings.Contains(pattern, sep) && part == pattern {
				return true
			}
		}
	}
	return false
}
