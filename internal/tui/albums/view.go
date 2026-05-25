package albums

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the albums panel.
type Level int

const (
	AlbumLevel Level = iota // showing the flat list of all albums
	TrackLevel              // showing tracks for a selected album
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the albums panel with 2-level drill-down:
// Albums -> Tracks. All fields are set by the caller before each Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Albums []string
	Tracks []library.Track

	// AlbumCounts maps each album name to its track count for the right-aligned
	// count column at AlbumLevel. Optional — when nil the count column is omitted.
	AlbumCounts map[string]int

	SelectedAlbum string

	// Saved cursor/scroll for album level (restored on Back).
	AlbumCursor       int
	AlbumScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display
	FilterHint   string // shown instead of cursor when filter is active and empty

	StripViewLabel     string // label rendered in the state strip; defaults to "Albums"
	StripSortLabel     string // sort field label for the strip; empty when sort inactive
	StripSortIndicator string // sort direction arrow for the strip; "▲" or "▼"

	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon  string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
}

// EffectiveAlbums returns the album list filtered by FilterText.
func (v View) EffectiveAlbums() []string { return arttui.FilterStrings(v.Albums, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track {
	return arttui.FilterTracks(v.Tracks, v.FilterText)
}

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve accounts for the state strip (which now carries the breadcrumb
// in place of a separate heading) and (at the track level) the column
// header:
//   - AlbumLevel: 1 (strip)
//   - TrackLevel: 2 (strip + column header)
func (v View) visibleRows() int {
	reserve := 1
	if v.Level == TrackLevel {
		reserve = 2
	}
	return layout.VisibleRows(v.Height, reserve)
}

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	if v.itemCount() == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor > 0 {
		v.Cursor--
	}
	return v.keepCursorVisible()
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	if v.itemCount() == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	limit := v.itemCount() - 1
	if v.Cursor < limit {
		v.Cursor++
	}
	return v.keepCursorVisible()
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

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	switch v.Level {
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return len(v.EffectiveAlbums())
	}
}

// moveCursor shifts the cursor by delta rows, clamping to [0, itemCount-1].
func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	return v.keepCursorVisible()
}

// keepCursorVisible adjusts ScrollOffset so the current cursor remains inside
// the visible window. When no rows fit, anchor the scroll to the cursor rather
// than advancing past it, so a later resize does not strand the selection.
func (v View) keepCursorVisible() View {
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
		if v.ScrollOffset < 0 {
			v.ScrollOffset = 0
		}
		return v
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks loaded.
// The current album cursor and scroll offset are saved for restoration by Back.
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

// Back returns a copy of v navigated one level up. At TrackLevel it returns to
// AlbumLevel. At AlbumLevel it is a no-op. The restored cursor/scroll are
// clamped against the parent list size so that mutations (e.g. a deleted
// album) during the drill-down do not leave the cursor pointing past the new
// end.
func (v View) Back() View {
	if v.Level == TrackLevel {
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
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	// Keep the cursor inside the visible window when the saved ScrollOffset
	// trailed far behind a (now clamped) Cursor.
	maxRows := v.visibleRows()
	if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// ---------- Data derivation (pure functions) ----------

// DeriveAllAlbums extracts unique album names from a track slice, sorted
// alphabetically. This collects albums across all artists.
func DeriveAllAlbums(tracks []library.Track) []string {
	seen := make(map[string]struct{})
	var albums []string
	for _, t := range tracks {
		album := t.Album
		if album == "" {
			album = arttui.UnknownAlbum
		}
		if _, ok := seen[album]; !ok {
			seen[album] = struct{}{}
			albums = append(albums, album)
		}
	}
	sort.Slice(albums, func(i, j int) bool {
		if albums[i] == arttui.UnknownAlbum {
			return false
		}
		if albums[j] == arttui.UnknownAlbum {
			return true
		}
		return albums[i] < albums[j]
	})
	return albums
}

// DeriveAllAlbumCounts maps each album name to its track count across the
// entire catalog. Tracks with an empty Album field count under the
// arttui.UnknownAlbum display sentinel, which also collapses any literal
// sentinel-valued album into the same rendered row.
func DeriveAllAlbumCounts(tracks []library.Track) map[string]int {
	if len(tracks) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, t := range tracks {
		album := t.Album
		if album == "" {
			album = arttui.UnknownAlbum
		}
		counts[album]++
	}
	return counts
}

// DeriveAlbumTracks returns all tracks matching the given album name,
// regardless of artist.
func DeriveAlbumTracks(tracks []library.Track, album string) []library.Track {
	rawAlbum := arttui.RawName(album)
	var result []library.Track
	for _, t := range tracks {
		if arttui.AlbumMatches(t.Album, rawAlbum) {
			result = append(result, t)
		}
	}
	return result
}
