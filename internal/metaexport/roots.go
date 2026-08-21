package metaexport

import (
	"path/filepath"
	"sort"
	"strings"
)

// Roots resolves absolute track paths against the configured music
// directories so exported rows can carry a root id plus a relative path.
type Roots struct {
	dirs  []string // cleaned, in configured order; id is index+1
	order []int    // indices into dirs, longest first
}

// NewRoots cleans dirs and indexes them longest-first. Paths are never
// resolved through the filesystem: EvalSymlinks would stat once per track and
// the scanner stores unresolved paths anyway, so a resolved root would stop
// matching them.
func NewRoots(dirs []string) Roots {
	cleaned := make([]string, len(dirs))
	order := make([]int, 0, len(dirs))
	for i, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue // keeps the id slot, but matches nothing
		}
		cleaned[i] = filepath.Clean(d)
		order = append(order, i)
	}
	sort.SliceStable(order, func(a, b int) bool {
		return len(cleaned[order[a]]) > len(cleaned[order[b]])
	})
	return Roots{dirs: cleaned, order: order}
}

// Dirs returns the cleaned roots in configured order, ready for Document.Roots.
// The id of dirs[i] is i+1.
func (r Roots) Dirs() []string { return r.dirs }

// Locate reports which root contains abs and the path relative to it, such
// that filepath.Join(root, rel) == abs. An unmatched path gets id 0, which the
// format reserves to mean "the path column is absolute".
//
// The longest root wins so a root nested inside another one still claims its
// own tracks, and the boundary is checked explicitly: a bare prefix test would
// let /Music swallow /Music2.
func (r Roots) Locate(abs string) (int, string) {
	for _, i := range r.order {
		root := r.dirs[i]
		if abs == root {
			return i + 1, ""
		}
		if strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return i + 1, abs[len(root)+1:]
		}
	}
	return 0, abs
}
