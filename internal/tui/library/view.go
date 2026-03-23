package library

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// QueueAppender appends a track to the persisted play queue. Implementations
// must be safe to call from a tea.Cmd goroutine (i.e., they must not hold the
// main-loop lock). The DB single-writer goroutine satisfies this contract.
// AppendTrack returns the new queue row ID assigned by the database.
// sourcePlaylist identifies the playlist the track was enqueued from; pass ""
// for tracks enqueued individually.
type QueueAppender interface {
	AppendTrack(track musiclib.Track, sourcePlaylist string) (int64, error)
}

// TrackEnqueuedMsg is returned by the tea.Cmd created by EnqueueFocused when
// the DB write completes. The root model handles it by appending the entry to
// the queue view and, when WasEmpty is true and playback is idle, triggering
// immediate playback from index 0.
type TrackEnqueuedMsg struct {
	Track          musiclib.Track
	QueueID        int64 // DB-assigned row ID; needed by the player for PlayQueueEntry
	Err            error
	WasEmpty       bool   // true when the queue was empty before this enqueue
	SourcePlaylist string // playlist name this track was enqueued from; empty for library enqueues
}

// View renders the music library panel with proportional columnar layout.
// Width and Height must be set from the most recent tea.WindowSizeMsg before
// calling Render; a zero Width falls back to a simple "artist – title" format.
// Cursor holds the zero-based index of the focused track row.
// Filter holds the active/text state of the filter bar.
// FilteredMatches is the pre-computed result of the last fuzzy search; callers
// should populate this whenever Filter.Text changes so that navigation and
// rendering reuse the cached slice without re-running the search on every frame.
// Fuzzy is the fuzzy search engine used when the filter bar is active;
// inject musiclib.FakeFuzzySearcher in tests and musiclib.SahilmFuzzySearcher in production.
// SelectedHashes holds the set of track hashes that are bulk-selected.
//
// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

type View struct {
	Index           musiclib.LibraryIndex
	Width           int
	Height          int
	Cursor          int
	ScrollOffset    int // first visible track index
	Filter          filter.State
	FilteredMatches []musiclib.Match // pre-computed; nil means not yet cached
	Fuzzy           musiclib.FuzzySearcher
	SelectedHashes  map[string]struct{}
	FavoriteHashes  map[string]struct{}
	Styles          styles.Styles
	OverrideTracks  []musiclib.Track // sorted tracks override; nil uses Index
	SortField       int              // active sort field (0 = default/none)
	SortDir         int              // 0 = asc, 1 = desc
}

// tracks returns the override tracks if set, otherwise the index tracks.
func (v View) tracks() []musiclib.Track {
	if v.OverrideTracks != nil {
		return v.OverrideTracks
	}
	return v.Index.Tracks()
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
// Reserve 1 line for the column header.
func (v View) visibleRows() int {
	return layout.VisibleRows(v.Height, 1)
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
		return v.Fuzzy.Search(v.Filter.Text, allTracks), true
	}
	return nil, false
}

// effectiveTrackCount returns the number of tracks currently displayed,
// accounting for any active fuzzy filter.
func (v View) effectiveTrackCount() int {
	if v.Filter.Text != "" {
		if m, ok := v.filteredMatches(v.tracks()); ok {
			return len(m)
		}
	}
	return len(v.tracks())
}

// FocusedTrack returns the track currently under the cursor, accounting for
// any active fuzzy filter. The second return value is false when the library
// is empty or the cursor is out of range.
func (v View) FocusedTrack() (musiclib.Track, bool) {
	allTracks := v.tracks()
	if v.Filter.Text != "" {
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
		id, err := qa.AppendTrack(track, "")
		return TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
}

// displayTrack pairs a track with its fuzzy match positions for highlight rendering.
type displayTrack struct {
	track           musiclib.Track
	matchedIndexes  []int
	artistPositions []int
	titlePositions  []int
	albumPositions  []int
}

// Render returns the complete library panel string. When Width is zero (before
// the first terminal size message) it uses a simple single-line format.
func (v View) Render() string {
	allTracks := v.tracks()
	n := len(allTracks)
	if n == 0 {
		return "(no tracks)"
	}

	// Resolve the set of tracks to display, with optional match positions.
	var display []displayTrack
	if v.Filter.Text != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			display = make([]displayTrack, len(matches))
			for i, m := range matches {
				display[i] = displayTrack{
					track:           allTracks[m.TrackIndex],
					matchedIndexes:  m.MatchedIndexes,
					artistPositions: m.ArtistPositions,
					titlePositions:  m.TitlePositions,
					albumPositions:  m.AlbumPositions,
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

	separator := ""
	if v.Filter.Active {
		ft := v.Filter.Text
		if ft == "" {
			hint := v.Styles.Dim.Render("a: artist  t: title  l: album")
			separator = "Filter: > " + hint + "\n"
		} else {
			maxLen := v.Width - len("Filter: > ") - 1
			if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
				ft = ansi.Truncate(ft, maxLen-1, "…")
			}
			separator = "Filter: > " + ft + "\n"
		}
	} else if v.Filter.Text != "" {
		ft := v.Filter.Text
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		separator = "Filter: " + ft + "\n"
	}

	if v.Width == 0 {
		if start >= end {
			return separator + "No matches — press Esc to clear"
		}
		lines := make([]string, end-start)
		for i, dt := range display[start:end] {
			absIdx := start + i
			row := fmt.Sprintf("%s – %s", dt.track.Artist, dt.track.Title)
			if absIdx == v.Cursor {
				row = v.Styles.Accent.Render(row)
			}
			lines[i] = row
		}
		return separator + strings.Join(lines, "\n")
	}

	cols := Compute(v.Width)

	// Append sort indicator to the active column header.
	artistH, titleH, albumH, timeH := "Artist", "Title", "Album", "Time"
	if v.SortField != 0 {
		ind := "▲"
		if v.SortDir == 1 {
			ind = "▼"
		}
		switch v.SortField {
		case 1: // SortArtist
			artistH += ind
		case 2: // SortTitle
			titleH += ind
		case 3: // SortAlbum
			albumH += ind
		case 4: // SortDuration
			timeH += ind
		}
	}
	header := v.Styles.ColHead.Render(strings.Repeat(" ", cols.SelectionMarker) + renderRow("#", artistH, titleH, albumH, timeH, cols))

	if start >= end {
		noMatches := "No matches"
		leftPad := max(0, (v.Width-len(noMatches))/2)
		return separator + header + "\n" + strings.Repeat(" ", leftPad) + noMatches
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
		// inner ANSI reset would reset the cursor highlight mid-row.
		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● " // plain glyph; outer accent wrap provides styling
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		default:
			markerStr = "  "
		}

		var row string
		hasFieldPos := len(dt.artistPositions) > 0 || len(dt.titlePositions) > 0 || len(dt.albumPositions) > 0
		switch {
		case hasFieldPos:
			row = markerStr + pad(num, cols.TrackNum) + " " +
				padHighlight(t.Artist, cols.Artist, dt.artistPositions) + " " +
				padHighlight(t.Title, cols.Title, dt.titlePositions) + " " +
				padHighlight(t.Album, cols.Album, dt.albumPositions) + " " +
				padLeft(dur, cols.Duration)
		case len(dt.matchedIndexes) > 0:
			artistOffset, artistLen, titleOffset, titleLen, albumOffset, albumLen :=
				searchFieldOffsetsAll(t.Artist, t.Title, t.Album)
			artistPos := fieldMatchPositions(dt.matchedIndexes, artistOffset, artistLen)
			titlePos := fieldMatchPositions(dt.matchedIndexes, titleOffset, titleLen)
			albumPos := fieldMatchPositions(dt.matchedIndexes, albumOffset, albumLen)
			row = markerStr + pad(num, cols.TrackNum) + " " +
				padHighlight(t.Artist, cols.Artist, artistPos) + " " +
				padHighlight(t.Title, cols.Title, titlePos) + " " +
				padHighlight(t.Album, cols.Album, albumPos) + " " +
				padLeft(dur, cols.Duration)
		default:
			row = markerStr + renderRow(num, t.Artist, t.Title, t.Album, dur, cols)
		}

		_, isFav := v.FavoriteHashes[t.Hash]
		switch {
		case isFocused && isFav:
			row = v.Styles.FavoriteAccent.Render(row)
		case isFocused:
			row = v.Styles.Accent.Render(row)
		case isFav:
			row = v.Styles.Favorite.Render(row)
		}
		lines[i] = row
	}

	return separator + header + "\n" + strings.Join(lines, "\n")
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
		padLeft(duration, cols.Duration)
}

// padLeft returns s truncated or left-padded with spaces to exactly width runes
// (i.e. right-aligned). Strings longer than width are truncated to width-1
// runes followed by "…".
func padLeft(s string, width int) string {
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
	return strings.Repeat(" ", width-n) + s
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
