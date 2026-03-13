package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

const maxDisplayedTracks = 50

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index library.LibraryIndex
}

// render returns a string representation of the library panel. It shows the
// track count in the header and up to maxDisplayedTracks entries as
// "Artist – Title" lines.
func (v LibraryView) render() string {
	n := v.index.Len()
	if n == 0 {
		return "Library\n\n(no tracks)"
	}
	tracks := v.index.Tracks()
	end := n
	if end > maxDisplayedTracks {
		end = maxDisplayedTracks
	}
	lines := make([]string, end)
	for i, t := range tracks[:end] {
		lines[i] = fmt.Sprintf("%s – %s", t.Artist, t.Title)
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
