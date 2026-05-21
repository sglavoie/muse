package library

import (
	"path/filepath"
	"strings"
)

// ShouldExclude reports whether path should be excluded based on the
// excludedPaths patterns. A pattern containing a path separator is treated as
// a directory prefix: it matches when path starts with pattern+"/" or equals
// the pattern exactly. A pattern without a separator is matched exactly
// (case-sensitive) against every component in path (both directory segments
// and the final element). Returns false immediately when excludedPaths is empty.
func ShouldExclude(path string, excludedPaths []string) bool {
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
		if strings.HasPrefix(path, clean+sep) || path == clean {
			return true
		}
	}

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
