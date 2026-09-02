package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// syncRecentlyAddedFromLibrary refreshes the library view's source tracks when
// the Library is in Recent mode. It is a no-op in other modes (data is loaded
// lazily when the user switches to that mode).
func (m Model) syncRecentlyAddedFromLibrary() Model {
	if m.library.mode != LibraryModeRecent {
		return m
	}
	m.library.sourceTracks = m.library.index.Tracks()
	if m.library.fuzzy == nil {
		m.library.fuzzy = library.SahilmFuzzySearcher{}
	}
	if m.library.sortState.Field == sorting.SortDefault {
		m.library.sortState = sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}
	}
	m.library = m.library.refreshView().clampCursor()
	return m
}

// libraryStripLabel returns the view-label string shown in the state
// strip for the given library mode. The label tracks the title bar
// abbreviation so users see a consistent view name in both places.
func libraryStripLabel(mode LibraryMode) string {
	switch mode {
	case LibraryModeRecent:
		return "Recent"
	case LibraryModeHistory:
		return "History"
	default:
		return "Library"
	}
}

// formatCount formats n with comma separators (e.g. 1234 → "1,234").
func formatCount(n int) string {
	raw := fmt.Sprintf("%d", n)
	sign := ""
	digits := raw
	if len(raw) > 0 && raw[0] == '-' {
		sign = "-"
		digits = raw[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	offset := len(digits) % 3
	var result []byte
	for i := range len(digits) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digits[i])
	}
	return sign + string(result)
}
