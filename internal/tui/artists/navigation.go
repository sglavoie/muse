package artists

import (
	"strings"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// FilterStrings returns items whose lowercased form contains the lowercased
// query. Returns items unchanged when query is empty.
func FilterStrings(items []string, query string) []string {
	if query == "" {
		return items
	}
	q := strings.ToLower(query)
	var out []string
	for _, s := range items {
		if strings.Contains(strings.ToLower(s), q) {
			out = append(out, s)
		}
	}
	return out
}

// FilterTracks returns tracks whose title, artist, or album contains query
// (case-insensitive). Returns tracks unchanged when query is empty.
func FilterTracks(tracks []library.Track, query string) []library.Track {
	if query == "" {
		return tracks
	}
	q := strings.ToLower(query)
	var out []library.Track
	for _, t := range tracks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Album), q) {
			out = append(out, t)
		}
	}
	return out
}

// EffectiveArtists returns the artist list filtered by FilterText.
func (v View) EffectiveArtists() []string { return FilterStrings(v.Artists, v.FilterText) }

// EffectiveAlbums returns the album list filtered by FilterText.
func (v View) EffectiveAlbums() []string { return FilterStrings(v.Albums, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track { return FilterTracks(v.Tracks, v.FilterText) }

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve accounts for the state strip (which now carries the breadcrumb
// in place of a separate heading) and any column header:
//   - ArtistLevel: 1 (strip)
//   - AlbumLevel:  1 (strip)
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

// CursorUp returns a copy of v with the cursor moved up by one row,
// stopping at the first entry. The scroll offset is adjusted so the
// cursor stays visible.
func (v View) CursorUp() View {
	return v.moveCursor(-1)
}

// CursorDown returns a copy of v with the cursor moved down by one row,
// stopping at the last entry. The scroll offset is adjusted so the
// cursor stays visible.
func (v View) CursorDown() View {
	return v.moveCursor(1)
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
	case AlbumLevel:
		return len(v.EffectiveAlbums())
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return len(v.EffectiveArtists())
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
	v.Cursor += delta
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

// OpenAlbums returns a copy of v transitioned to AlbumLevel with albums loaded.
// The current artist cursor and scroll offset are saved so they can be restored
// by Back.
func (v View) OpenAlbums(albums []string) View {
	v.ArtistCursor = v.Cursor
	v.ArtistScrollOffset = v.ScrollOffset
	v.Albums = albums
	v.Level = AlbumLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks loaded.
// The current album cursor and scroll offset are saved so they can be restored
// by Back.
func (v View) OpenTracks(tracks []library.Track) View {
	v.AlbumCursor = v.Cursor
	v.AlbumScrollOffset = v.ScrollOffset
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// Back returns a copy of v navigated one level up. At AlbumLevel it returns to
// ArtistLevel; at TrackLevel it returns to AlbumLevel. At ArtistLevel it is a
// no-op. The restored cursor/scroll are clamped against the parent list size
// so that mutations (e.g. a deleted artist/album) during the drill-down do not
// leave the cursor pointing past the new end.
func (v View) Back() View {
	switch v.Level {
	case AlbumLevel:
		v.Level = ArtistLevel
		v.Cursor = v.ArtistCursor
		v.ScrollOffset = v.ArtistScrollOffset
		v.Albums = nil
		v.FilterActive = false
		v.FilterText = ""
		v = v.clampRestoredCursor()
	case TrackLevel:
		v.Level = AlbumLevel
		v.Cursor = v.AlbumCursor
		v.ScrollOffset = v.AlbumScrollOffset
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
	maxRows := v.visibleRows()
	if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}
