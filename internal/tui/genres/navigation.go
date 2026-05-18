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
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	limit := v.itemCount() - 1
	if v.Cursor < limit {
		v.Cursor++
	}
	maxRows := v.visibleRows()
	if v.Cursor >= v.ScrollOffset+maxRows {
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
	if n > 0 {
		v.Cursor = n - 1
	}
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
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

// OpenArtists returns a copy of v transitioned to ArtistLevel with artists
// loaded. The current genre cursor and scroll offset are saved so they can be
// restored by Back.
func (v View) OpenArtists(artists []string) View {
	v.GenreCursor = v.Cursor
	v.GenreScrollOffset = v.ScrollOffset
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
// by Back.
func (v View) OpenTracks(tracks []library.Track) View {
	v.ArtistCursor = v.Cursor
	v.ArtistScrollOffset = v.ScrollOffset
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// Back returns a copy of v navigated one level up. At ArtistLevel it returns to
// GenreLevel; at TrackLevel it returns to ArtistLevel. At GenreLevel it is a
// no-op.
func (v View) Back() View {
	switch v.Level {
	case ArtistLevel:
		v.Level = GenreLevel
		v.Cursor = v.GenreCursor
		v.ScrollOffset = v.GenreScrollOffset
		v.Artists = nil
		v.FilterActive = false
		v.FilterText = ""
	case TrackLevel:
		v.Level = ArtistLevel
		v.Cursor = v.ArtistCursor
		v.ScrollOffset = v.ArtistScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
	}
	return v
}
