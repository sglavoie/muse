package library

import (
	"fmt"
	"strings"

	musiclib "github.com/sglavoie/muse/internal/library"
)

// ansiAccent / ansiReset wrap text in reverse-video for the focused-row highlight.
const (
	ansiAccent = "\033[7m"
	ansiReset  = "\033[0m"
)

// View renders the music library panel with proportional columnar layout.
// Width and Height must be set from the most recent tea.WindowSizeMsg before
// calling Render; a zero Width falls back to a simple "artist – title" format.
// Cursor holds the zero-based index of the focused track row.
// FilterActive is false by default and is set to true when the user presses '/'.
// FilterText accumulates the query string while FilterActive is true.
// Fuzzy is the fuzzy search engine used when the filter bar is active;
// inject musiclib.FakeFuzzySearcher in tests and musiclib.SahilmFuzzySearcher in production.
type View struct {
	Index        musiclib.LibraryIndex
	Width        int
	Height       int
	Cursor       int
	FilterActive bool
	FilterText   string
	Fuzzy        musiclib.FuzzySearcher
}

// CursorUp moves the cursor one row up, stopping at the first track.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	return v
}

// CursorDown moves the cursor one row down, stopping at the last track.
func (v View) CursorDown() View {
	n := v.effectiveTrackCount()
	if n > 0 && v.Cursor < n-1 {
		v.Cursor++
	}
	return v
}

// effectiveTrackCount returns the number of tracks currently displayed,
// accounting for any active fuzzy filter.
func (v View) effectiveTrackCount() int {
	if v.FilterActive && v.Fuzzy != nil {
		return len(v.Fuzzy.Search(v.FilterText, v.Index.Tracks()))
	}
	return v.Index.Len()
}

// displayTrack pairs a track with its fuzzy match positions for highlight rendering.
type displayTrack struct {
	track          musiclib.Track
	matchedIndexes []int
}

// Render returns the complete library panel string. When Width is zero (before
// the first terminal size message) it uses a simple single-line format.
func (v View) Render() string {
	n := v.Index.Len()
	if n == 0 {
		return "Library\n\n(no tracks)"
	}

	allTracks := v.Index.Tracks()

	// Resolve the set of tracks to display, with optional match positions.
	var display []displayTrack
	if v.FilterActive && v.Fuzzy != nil {
		matches := v.Fuzzy.Search(v.FilterText, allTracks)
		display = make([]displayTrack, len(matches))
		for i, m := range matches {
			display[i] = displayTrack{
				track:          allTracks[m.TrackIndex],
				matchedIndexes: m.MatchedIndexes,
			}
		}
	} else {
		display = make([]displayTrack, len(allTracks))
		for i, t := range allTracks {
			display[i] = displayTrack{track: t}
		}
	}

	reserve := 3
	if v.FilterActive {
		reserve = 4
	}
	maxRows := 50
	if v.Height > 0 {
		maxRows = max(1, v.Height-reserve)
	}
	end := min(len(display), maxRows)

	heading := fmt.Sprintf("Library (%s tracks)", formatCount(n))
	separator := "\n\n"
	if v.FilterActive {
		separator = "\nFilter: > " + v.FilterText + "\n"
	}

	if v.Width == 0 {
		if end == 0 {
			return heading + separator + "(no matches)"
		}
		lines := make([]string, end)
		for i, dt := range display[:end] {
			row := fmt.Sprintf("%s – %s", dt.track.Artist, dt.track.Title)
			if i == v.Cursor {
				row = ansiAccent + row + ansiReset
			}
			lines[i] = row
		}
		return heading + separator + strings.Join(lines, "\n")
	}

	cols := Compute(v.Width)
	header := renderRow("#", "Artist", "Title", "Album", "Time", cols)

	if end == 0 {
		return heading + separator + header + "\n(no matches)"
	}

	lines := make([]string, end)
	for i, dt := range display[:end] {
		t := dt.track
		num := fmt.Sprintf("%d", i+1)
		dur := formatDuration(t.Duration)

		var row string
		if len(dt.matchedIndexes) > 0 {
			artistRunes := len([]rune(t.Artist))
			titleRunes := len([]rune(t.Title))
			artistPos := fieldMatchPositions(dt.matchedIndexes, 0, artistRunes)
			titlePos := fieldMatchPositions(dt.matchedIndexes, artistRunes+1, titleRunes)
			row = pad(num, cols.TrackNum) + " " +
				padHighlight(t.Artist, cols.Artist, artistPos) + " " +
				padHighlight(t.Title, cols.Title, titlePos) + " " +
				pad(t.Album, cols.Album) + " " +
				pad(dur, cols.Duration)
		} else {
			row = renderRow(num, t.Artist, t.Title, t.Album, dur, cols)
		}

		if i == v.Cursor {
			row = ansiAccent + row + ansiReset
		}
		lines[i] = row
	}

	return heading + separator + header + "\n" + strings.Join(lines, "\n")
}

// renderRow formats a single row with each field padded or truncated to its
// column width. When cols.Artist is zero the layout degenerates to just the
// track number field.
func renderRow(num, artist, title, album, duration string, cols Columns) string {
	if cols.Artist == 0 {
		return pad(num, cols.TrackNum)
	}
	return pad(num, cols.TrackNum) + " " +
		pad(artist, cols.Artist) + " " +
		pad(title, cols.Title) + " " +
		pad(album, cols.Album) + " " +
		pad(duration, cols.Duration)
}

// pad returns s truncated or right-padded with spaces to exactly width runes.
// Strings longer than width are truncated to width-1 runes followed by "…".
func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	n := len(runes)
	if n > width {
		if width == 1 {
			return string(runes[:1])
		}
		return string(runes[:width-1]) + "…"
	}
	if n == width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// formatDuration converts a duration in seconds to M:SS or H:MM:SS format.
// Returns "--:--" when secs is zero or negative (unknown duration).
func formatDuration(secs float64) string {
	if secs <= 0 {
		return "--:--"
	}
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
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
