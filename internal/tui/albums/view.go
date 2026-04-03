package albums

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
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

	SortIndicator   string // e.g. "Sort: Name ▲" — shown when non-empty
	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
}

// EffectiveAlbums returns the album list filtered by FilterText.
func (v View) EffectiveAlbums() []string { return arttui.FilterStrings(v.Albums, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track {
	return arttui.FilterTracks(v.Tracks, v.FilterText)
}

// visibleRows returns the maximum number of rows that fit on screen.
// The reserve depends on the current level: top-level album list has no
// chrome (0), while the track level has a heading + separator line (2).
// An active or committed filter adds 1 line at the top level.
// A sort indicator adds 1 line at any level.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case AlbumLevel:
		if v.FilterActive || v.FilterText != "" {
			reserve = 1
		}
	default:
		reserve = 3
	}
	if v.SortIndicator != "" {
		reserve++
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
// AlbumLevel. At AlbumLevel it is a no-op.
func (v View) Back() View {
	if v.Level == TrackLevel {
		v.Level = AlbumLevel
		v.Cursor = v.AlbumCursor
		v.ScrollOffset = v.AlbumScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
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

// DeriveAlbumTracks returns all tracks matching the given album name,
// regardless of artist.
func DeriveAlbumTracks(tracks []library.Track, album string) []library.Track {
	album = arttui.RawName(album)
	var result []library.Track
	for _, t := range tracks {
		if t.Album == album {
			result = append(result, t)
		}
	}
	return result
}

// ---------- Rendering ----------

// Render returns the string representation of the albums panel.
func (v View) Render() string {
	switch v.Level {
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderAlbums()
	}
}

func (v View) renderAlbums() string {
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return "(no albums)"
	}
	if len(items) == 0 {
		return v.topFilterSeparator() + "(no matches)"
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteAlbums[name]
		display := text.Pad(name, v.Width)
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavAlbumAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = v.Styles.FavAlbum.Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render(v.SelectedAlbum)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)

	header := v.Styles.ColHead.Render(
		strings.Repeat(" ", cols.SelectionMarker) +
			fmt.Sprintf("%-*s %-*s %-*s %*s",
				cols.Position, "#", cols.Title, "Title", cols.Artist, "Artist", cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		_, isSelected := v.SelectedHashes[t.Hash]

		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● " // plain glyph; outer accent wrap provides styling
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%-*d", cols.Position, absIdx+1)
		titleStr := text.Pad(t.Title, cols.Title)
		artistStr := text.Pad(t.Artist, cols.Artist)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		if isFocused {
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + a.Render(dur)
		} else {
			if isTrackFav {
				titleStr = v.Styles.Favorite.Render(titleStr)
			}
			if isArtistFav {
				artistStr = v.Styles.FavArtist.Render(artistStr)
			}
			lines[i] = markerStr + pos + " " + titleStr + " " + artistStr + " " + dur
		}
	}
	return heading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	var prefix string
	if v.SortIndicator != "" {
		prefix = v.SortIndicator + "\n"
	}
	if v.FilterActive {
		fv := v.FilterView
		if fv == "" {
			fv = v.FilterText
		}
		return prefix + "Filter: " + fv + "\n"
	}
	if v.FilterText != "" {
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return prefix + "Filter: " + ft + "\n"
	}
	return prefix
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	var sortLine string
	if v.SortIndicator != "" {
		sortLine = "\n" + v.SortIndicator
	}
	if v.FilterActive {
		fv := v.FilterView
		if fv == "" {
			fv = v.FilterText
		}
		return sortLine + "\nFilter: " + fv + "\n"
	}
	if v.FilterText != "" {
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return sortLine + "\nFilter: " + ft + "\n"
	}
	return sortLine + "\n\n"
}
