package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index  library.LibraryIndex
	width  int
	height int
}

// render returns a string representation of the library panel. It shows the
// track count in the header and up to as many entries as the terminal height
// allows. Three rows are reserved for the header line, a blank line after it,
// and a blank line before the status bar (header + 2 padding lines = 3).
// When v.height is not yet known the list is capped at 50 rows as a safe
// default until the first tea.WindowSizeMsg arrives.
//
// TODO: replace the slice cap with a proper scroll/pagination mechanism so the
// user can navigate libraries larger than the terminal height.
func (v LibraryView) render() string {
	n := v.index.Len()
	if n == 0 {
		return "Library\n\n(no tracks)"
	}
	tracks := v.index.Tracks()
	maxRows := 50
	if v.height > 0 {
		maxRows = max(1, v.height-3)
	}
	end := min(n, maxRows)
	lines := make([]string, end)
	for i, t := range tracks[:end] {
		line := fmt.Sprintf("%s – %s", t.Artist, t.Title)
		if v.width > 0 {
			line = truncateTitle(line, v.width)
		}
		lines[i] = line
	}
	return fmt.Sprintf("Library (%s tracks)\n\n%s", formatCount(n), strings.Join(lines, "\n"))
}

// formatCount formats n with comma separators (e.g. 1234 → "1,234").
func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	offset := len(s) % 3
	var result []byte
	for i := range len(s) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, s[i])
	}
	return string(result)
}
