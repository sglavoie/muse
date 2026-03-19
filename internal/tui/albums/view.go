package albums

import (
	"fmt"
	"sort"
	"strings"

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

	SelectedAlbum string

	// Saved cursor/scroll for album level (restored on Back).
	AlbumCursor       int
	AlbumScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string

	SortIndicator string // e.g. "Sort: Name ▲" — shown when non-empty
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
		reserve = 2
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
		display := arttui.Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		if absIdx == v.Cursor {
			display = v.Styles.Accent.Render(display)
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

	cols := arttui.ComputeTrack(v.Width)

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i

		pos := fmt.Sprintf("%-*d", cols.Position, absIdx+1)
		title := arttui.Truncate(t.Title, cols.Title)
		title = fmt.Sprintf("%-*s", cols.Title, title)
		dur := arttui.FormatDuration(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		line := pos + " " + title + " " + dur
		if absIdx == v.Cursor {
			line = v.Styles.Accent.Render(line)
		}
		lines[i] = line
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	var prefix string
	if v.SortIndicator != "" {
		prefix = v.SortIndicator + "\n"
	}
	if v.FilterActive {
		if v.FilterText == "" {
			return prefix + "Filter: > \n"
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: > ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
		}
		return prefix + "Filter: > " + ft + "\n"
	}
	if v.FilterText != "" {
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
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
		if v.FilterText == "" {
			return sortLine + "\nFilter: > \n"
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: > ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
		}
		return sortLine + "\nFilter: > " + ft + "\n"
	}
	if v.FilterText != "" {
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
		}
		return sortLine + "\nFilter: " + ft + "\n"
	}
	return sortLine + "\n\n"
}
