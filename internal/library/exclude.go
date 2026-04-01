package library

import (
	"path/filepath"
	"slices"
	"strings"
)

// ShouldExclude reports whether path should be excluded based on the
// excludedPaths patterns. Each pattern is matched exactly (case-sensitive)
// against every component in path (both directory segments and the final
// element). Returns false immediately when excludedPaths is empty.
func ShouldExclude(path string, excludedPaths []string) bool {
	if len(excludedPaths) == 0 {
		return false
	}
	for part := range strings.SplitSeq(path, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if slices.Contains(excludedPaths, part) {
			return true
		}
	}
	return false
}
