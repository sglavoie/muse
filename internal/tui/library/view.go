package library

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
)

// QueueAppender appends a track to the persisted play queue. Implementations
// must be safe to call from a tea.Cmd goroutine (i.e., they must not hold the
// main-loop lock). The DB single-writer goroutine satisfies this contract.
// AppendTrack returns the new queue row ID assigned by the database.
type QueueAppender interface {
	AppendTrack(track musiclib.Track) (int64, error)
}

// TrackEnqueuedMsg is returned by the tea.Cmd created by EnqueueFocused when
// the DB write completes. The root model handles it by appending the entry to
// the queue view and, when WasEmpty is true and playback is idle, triggering
// immediate playback from index 0.
type TrackEnqueuedMsg struct {
	Track    musiclib.Track
	QueueID  int64 // DB-assigned row ID; needed by the player for PlayQueueEntry
	Err      error
	WasEmpty bool // true when the queue was empty before this enqueue
}

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
// FilteredMatches is the pre-computed result of the last fuzzy search; callers
// should populate this whenever FilterText changes so that navigation and
// rendering reuse the cached slice without re-running the search on every frame.
// Fuzzy is the fuzzy search engine used when the filter bar is active;
// inject musiclib.FakeFuzzySearcher in tests and musiclib.SahilmFuzzySearcher in production.
// SelectedHashes holds the set of track hashes that are bulk-selected.
type View struct {
	Index           musiclib.LibraryIndex
	Width           int
	Height          int
	Cursor          int
	ScrollOffset    int // first visible track index
	FilterActive    bool
	FilterText      string
	FilteredMatches []musiclib.Match // pre-computed; nil means not yet cached
	Fuzzy           musiclib.FuzzySearcher
	SelectedHashes  map[string]struct{}
}

// CursorUp moves the cursor one row up, stopping at the first track.
// The scroll offset is adjusted so the cursor stays visible.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown moves the cursor one row down, stopping at the last track.
// The scroll offset is adjusted so the cursor stays visible.
func (v View) CursorDown() View {
	n := v.effectiveTrackCount()
	if n > 0 && v.Cursor < n-1 {
		v.Cursor++
	}
	maxRows := v.visibleRows()
	if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View {
	return v.moveCursor(v.visibleRows() / 2)
}

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View {
	return v.moveCursor(-v.visibleRows() / 2)
}

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View {
	return v.moveCursor(v.visibleRows())
}

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View {
	return v.moveCursor(-v.visibleRows())
}

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.effectiveTrackCount()
	if n > 0 {
		v.Cursor = n - 1
	}
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// moveCursor shifts the cursor by delta rows, clamping to [0, trackCount-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v View) moveCursor(delta int) View {
	n := v.effectiveTrackCount()
	if n == 0 {
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// visibleRows returns the maximum number of track rows that fit on screen.
func (v View) visibleRows() int {
	reserve := 3
	if v.FilterText != "" {
		reserve = 4
	}
	if v.Height > 0 {
		return max(1, v.Height-reserve)
	}
	return 50
}

// filteredMatches returns the current fuzzy-match slice and whether a filter
// searcher is available. The bool is true when FilteredMatches is populated or
// Fuzzy is non-nil (i.e. a filter can be applied). When the bool is false the
// caller should fall back to the full unfiltered track list.
func (v View) filteredMatches(allTracks []musiclib.Track) ([]musiclib.Match, bool) {
	if v.FilteredMatches != nil {
		return v.FilteredMatches, true
	}
	if v.Fuzzy != nil {
		return v.Fuzzy.Search(v.FilterText, allTracks), true
	}
	return nil, false
}

// effectiveTrackCount returns the number of tracks currently displayed,
// accounting for any active fuzzy filter.
func (v View) effectiveTrackCount() int {
	if v.FilterText != "" {
		if m, ok := v.filteredMatches(v.Index.Tracks()); ok {
			return len(m)
		}
	}
	return v.Index.Len()
}

// FocusedTrack returns the track currently under the cursor, accounting for
// any active fuzzy filter. The second return value is false when the library
// is empty or the cursor is out of range.
func (v View) FocusedTrack() (musiclib.Track, bool) {
	allTracks := v.Index.Tracks()
	if v.FilterText != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			if v.Cursor < 0 || v.Cursor >= len(matches) {
				return musiclib.Track{}, false
			}
			return allTracks[matches[v.Cursor].TrackIndex], true
		}
	}
	if v.Cursor < 0 || v.Cursor >= len(allTracks) {
		return musiclib.Track{}, false
	}
	return allTracks[v.Cursor], true
}

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. prevQueueLen is the number of entries in the queue before
// this call; it is forwarded in TrackEnqueuedMsg.WasEmpty so the root model
// can decide whether to trigger immediate playback. Returns nil Cmd when the
// library is empty or the cursor is out of range.
func (v View) EnqueueFocused(qa QueueAppender, prevQueueLen int) (View, tea.Cmd) {
	track, ok := v.FocusedTrack()
	if !ok {
		return v, nil
	}
	wasEmpty := prevQueueLen == 0
	return v, func() tea.Msg {
		id, err := qa.AppendTrack(track)
		return TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
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
	if v.FilterText != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			display = make([]displayTrack, len(matches))
			for i, m := range matches {
				display[i] = displayTrack{
					track:          allTracks[m.TrackIndex],
					matchedIndexes: m.MatchedIndexes,
				}
			}
		}
	}
	if display == nil {
		display = make([]displayTrack, len(allTracks))
		for i, t := range allTracks {
			display[i] = displayTrack{track: t}
		}
	}

	maxRows := v.visibleRows()
	start := v.ScrollOffset
	if start > len(display) {
		start = len(display)
	}
	end := min(len(display), start+maxRows)

	heading := fmt.Sprintf("Library (%s tracks)", formatCount(n))
	separator := "\n\n"
	if v.FilterActive {
		separator = "\nFilter: > " + v.FilterText + "\n"
	} else if v.FilterText != "" {
		separator = "\nFilter: " + v.FilterText + "\n"
	}

	if v.Width == 0 {
		if start >= end {
			return heading + separator + "No matches"
		}
		lines := make([]string, end-start)
		for i, dt := range display[start:end] {
			absIdx := start + i
			row := fmt.Sprintf("%s – %s", dt.track.Artist, dt.track.Title)
			if absIdx == v.Cursor {
				row = ansiAccent + row + ansiReset
			}
			lines[i] = row
		}
		return heading + separator + strings.Join(lines, "\n")
	}

	cols := Compute(v.Width)
	header := strings.Repeat(" ", cols.SelectionMarker) + renderRow("#", "Artist", "Title", "Album", "Time", cols)

	if start >= end {
		noMatches := "No matches"
		leftPad := max(0, (v.Width-len(noMatches))/2)
		return heading + separator + header + "\n" + strings.Repeat(" ", leftPad) + noMatches
	}

	lines := make([]string, end-start)
	for i, dt := range display[start:end] {
		absIdx := start + i
		t := dt.track
		num := fmt.Sprintf("%d", absIdx+1)
		dur := formatDuration(t.Duration)

		_, isSelected := v.SelectedHashes[t.Hash]
		isFocused := absIdx == v.Cursor

		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer reverse-video wrap covers the full row uniformly. An
		// inner \033[0m would reset the cursor highlight mid-row.
		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● " // plain glyph; outer accent wrap provides styling
		case isSelected:
			markerStr = ansiAccent + "●" + ansiReset + " "
		default:
			markerStr = "  "
		}

		var row string
		if len(dt.matchedIndexes) > 0 {
			artistOffset, artistLen, titleOffset, titleLen := searchFieldOffsets(t.Artist, t.Title)
			artistPos := fieldMatchPositions(dt.matchedIndexes, artistOffset, artistLen)
			titlePos := fieldMatchPositions(dt.matchedIndexes, titleOffset, titleLen)
			row = markerStr + pad(num, cols.TrackNum) + " " +
				padHighlight(t.Artist, cols.Artist, artistPos) + " " +
				padHighlight(t.Title, cols.Title, titlePos) + " " +
				pad(t.Album, cols.Album) + " " +
				pad(dur, cols.Duration)
		} else {
			row = markerStr + renderRow(num, t.Artist, t.Title, t.Album, dur, cols)
		}

		if isFocused {
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
