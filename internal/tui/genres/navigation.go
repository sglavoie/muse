package genres

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve accounts for the state strip (which now carries the breadcrumb
// in place of a separate heading) and any column header:
//   - GenreLevel:  1 (strip)
//   - ArtistLevel: 1 (strip)
//   - TrackLevel:  2 (strip + column header)
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case TrackLevel:
		reserve = 2
	default:
		reserve = 1
	}
	return layout.VisibleRows(v.Height, reserve)
}

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	switch {
	case v.Cursor < 0:
		v.Cursor = 0
	case v.Cursor >= n:
		v.Cursor = n - 1
	case v.Cursor > 0:
		v.Cursor--
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	switch {
	case v.Cursor < 0:
		v.Cursor = 0
	case v.Cursor >= n:
		v.Cursor = n - 1
	case v.Cursor < n-1:
		v.Cursor++
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	maxRows := v.visibleRows()
	if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View { return v.moveCursor(v.visibleRows() / 2) }

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View { return v.moveCursor(-v.visibleRows() / 2) }

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View { return v.moveCursor(v.visibleRows()) }

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View { return v.moveCursor(-v.visibleRows()) }

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor = n - 1
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	// Clamp ScrollOffset down when the list shrank below a previously
	// scrolled position: otherwise the cursor renders above the visible
	// window after a filter/delete reduces itemCount.
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// itemCount returns the number of items in the current level, accounting for
// any active filter.
func (v View) itemCount() int {
	switch v.Level {
	case ArtistLevel:
		return len(v.EffectiveArtists())
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return len(v.EffectiveGenres())
	}
}

// moveCursor shifts the cursor by delta rows, clamping to [0, itemCount-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	switch {
	case delta > 0:
		limit := n - 1 - delta
		if limit < 0 || v.Cursor > limit {
			v.Cursor = n - 1
		} else {
			v.Cursor += delta
		}
	case delta < 0:
		step := -delta
		if v.Cursor < step {
			v.Cursor = 0
		} else {
			v.Cursor -= step
		}
	}
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// OpenArtists returns a copy of v transitioned to ArtistLevel with artists
// loaded. The current genre cursor and scroll offset are saved so they can be
// restored by Back. When a filter is active, the saved cursor is translated
// back to the unfiltered Genres index so the cursor lands on the same entry
// after Back clears the filter.
func (v View) OpenArtists(artists []string) View {
	v.GenreCursor, v.GenreScrollOffset = v.unfilteredGenreCursorPos()
	v.Artists = artists
	v.Level = ArtistLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks loaded.
// The current artist cursor and scroll offset are saved so they can be restored
// by Back. When a filter is active, the saved cursor is translated back to the
// unfiltered Artists index so the cursor lands on the same entry after Back
// clears the filter.
func (v View) OpenTracks(tracks []library.Track) View {
	v.ArtistCursor, v.ArtistScrollOffset = v.unfilteredArtistCursorPos()
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// unfilteredGenreCursorPos returns the cursor translated back to the
// unfiltered Genres slice when a filter is active. ScrollOffset is reset to 0
// in that case so clampRestoredCursor (run by Back) can re-establish a
// visible window for the restored cursor. Falls back to the raw values when
// no filter is active or the focused genre cannot be resolved.
func (v View) unfilteredGenreCursorPos() (int, int) {
	if v.FilterText == "" {
		return v.Cursor, v.ScrollOffset
	}
	items := v.EffectiveGenres()
	if len(items) == 0 || v.Cursor < 0 || v.Cursor >= len(items) {
		return v.Cursor, v.ScrollOffset
	}
	focused := items[v.Cursor]
	for i, name := range v.Genres {
		if name == focused {
			return i, 0
		}
	}
	return v.Cursor, v.ScrollOffset
}

// unfilteredArtistCursorPos returns the cursor translated back to the
// unfiltered Artists slice when a filter is active. ScrollOffset is reset to 0
// for the same reason as unfilteredGenreCursorPos. Falls back to the raw
// values when no filter is active or the focused artist cannot be resolved.
func (v View) unfilteredArtistCursorPos() (int, int) {
	if v.FilterText == "" {
		return v.Cursor, v.ScrollOffset
	}
	items := v.EffectiveArtists()
	if len(items) == 0 || v.Cursor < 0 || v.Cursor >= len(items) {
		return v.Cursor, v.ScrollOffset
	}
	focused := items[v.Cursor]
	for i, name := range v.Artists {
		if name == focused {
			return i, 0
		}
	}
	return v.Cursor, v.ScrollOffset
}

// Back returns a copy of v navigated one level up. At ArtistLevel it returns to
// GenreLevel; at TrackLevel it returns to ArtistLevel. At GenreLevel it is a
// no-op. The restored cursor/scroll are clamped against the parent list size
// so that mutations (e.g. a deleted genre/artist) during the drill-down do not
// leave the cursor pointing past the new end.
func (v View) Back() View {
	switch v.Level {
	case ArtistLevel:
		v.Level = GenreLevel
		v.Cursor = v.GenreCursor
		v.ScrollOffset = v.GenreScrollOffset
		v.Artists = nil
		v.FilterActive = false
		v.FilterText = ""
		v = v.clampRestoredCursor()
	case TrackLevel:
		v.Level = ArtistLevel
		v.Cursor = v.ArtistCursor
		v.ScrollOffset = v.ArtistScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v = v.clampRestoredCursor()
	}
	return v
}

// clampRestoredCursor clamps Cursor to [0, itemCount-1] and adjusts
// ScrollOffset to keep the cursor visible. Used after Back() restores a saved
// cursor that may now point past the (possibly shrunk) parent list.
func (v View) clampRestoredCursor() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	}
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	maxRows := v.visibleRows()
	if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}
