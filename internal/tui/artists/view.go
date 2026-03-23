package artists

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the artists panel.
type Level int

const (
	ArtistLevel Level = iota // showing the flat list of artists
	AlbumLevel               // showing albums for a selected artist
	TrackLevel               // showing tracks for a selected album
)

const (
	UnknownArtist = "(unknown artist)"
	UnknownAlbum  = "(unknown album)"
)

// RawName converts display sentinels back to an empty string for matching
// against raw track metadata. All other values pass through.
func RawName(s string) string {
	if s == UnknownArtist || s == UnknownAlbum {
		return ""
	}
	return s
}

// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the artists panel with 3-level drill-down:
// Artists -> Albums -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Artists []string
	Albums  []string
	Tracks  []library.Track

	SelectedArtist string
	SelectedAlbum  string

	// Saved cursors/scroll for each level (restored on Back).
	ArtistCursor       int
	ArtistScrollOffset int
	AlbumCursor        int
	AlbumScrollOffset  int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string

	SortIndicator  string // e.g. "Sort: Name ▲" — shown when non-empty
	FavoriteHashes map[string]struct{}
	SelectedHashes map[string]struct{}
}

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
// The reserve depends on the current level: top-level artist list has no
// chrome (0), while drill-down levels have a heading + separator line (2).
// An active or committed filter adds 1 line at the top level.
// A sort indicator adds 1 line at any level.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case ArtistLevel:
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

// CursorUp returns a copy of v with the cursor moved up by one row,
// stopping at the first entry. The scroll offset is adjusted so the
// cursor stays visible.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row,
// stopping at the last entry. The scroll offset is adjusted so the
// cursor stays visible.
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
// no-op.
func (v View) Back() View {
	switch v.Level {
	case AlbumLevel:
		v.Level = ArtistLevel
		v.Cursor = v.ArtistCursor
		v.ScrollOffset = v.ArtistScrollOffset
		v.Albums = nil
		v.FilterActive = false
		v.FilterText = ""
	case TrackLevel:
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

// DeriveArtists extracts unique artist names from a sorted track slice.
// The input must be sorted by artist (as LibraryIndex.Tracks() provides).
func DeriveArtists(tracks []library.Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	name := func(a string) string {
		if a == "" {
			return UnknownArtist
		}
		return a
	}
	artists := []string{name(tracks[0].Artist)}
	for i := 1; i < len(tracks); i++ {
		if tracks[i].Artist != tracks[i-1].Artist {
			artists = append(artists, name(tracks[i].Artist))
		}
	}
	return artists
}

// DeriveAlbums returns unique album names for the given artist, sorted
// alphabetically.
func DeriveAlbums(tracks []library.Track, artist string) []string {
	artist = RawName(artist)
	seen := make(map[string]struct{})
	var albums []string
	for _, t := range tracks {
		if t.Artist != artist {
			continue
		}
		album := t.Album
		if album == "" {
			album = UnknownAlbum
		}
		if _, ok := seen[album]; !ok {
			seen[album] = struct{}{}
			albums = append(albums, album)
		}
	}
	sort.Slice(albums, func(i, j int) bool {
		if albums[i] == UnknownAlbum {
			return false
		}
		if albums[j] == UnknownAlbum {
			return true
		}
		return albums[i] < albums[j]
	})
	return albums
}

// DeriveTracks returns all tracks matching the given artist and album.
func DeriveTracks(tracks []library.Track, artist, album string) []library.Track {
	artist = RawName(artist)
	album = RawName(album)
	var result []library.Track
	for _, t := range tracks {
		if t.Artist == artist && t.Album == album {
			result = append(result, t)
		}
	}
	return result
}

// DeriveArtistTracks returns all tracks by the given artist, regardless of album.
func DeriveArtistTracks(tracks []library.Track, artist string) []library.Track {
	raw := RawName(artist)
	var result []library.Track
	for _, t := range tracks {
		if t.Artist == raw {
			result = append(result, t)
		}
	}
	return result
}

// ---------- Rendering ----------

// Render returns the string representation of the artists panel.
func (v View) Render() string {
	switch v.Level {
	case AlbumLevel:
		return v.renderAlbums()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderArtists()
	}
}

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return "(no artists)"
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
		display := Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		if absIdx == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderAlbums() string {
	heading := v.Styles.Heading.Render(v.SelectedArtist)
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return heading + "\n\n(no albums)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		display := Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		if absIdx == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTracks() string {
	heading := v.SelectedArtist
	if v.SelectedAlbum != "" {
		heading += " — " + v.SelectedAlbum
	}
	styledHeading := v.Styles.Heading.Render(heading)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return styledHeading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return styledHeading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)

	header := v.Styles.ColHead.Render(
		strings.Repeat(" ", cols.SelectionMarker) +
			fmt.Sprintf("%-*s %-*s %*s",
				cols.Position, "#", cols.Title, "Title", cols.Duration, "Time"))

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
		title := Truncate(t.Title, cols.Title)
		title = fmt.Sprintf("%-*s", cols.Title, title)
		dur := FormatDuration(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		line := markerStr + pos + " " + title + " " + dur
		_, isFav := v.FavoriteHashes[t.Hash]
		switch {
		case isFocused && isFav:
			line = v.Styles.FavoriteAccent.Render(line)
		case isFocused:
			line = v.Styles.Accent.Render(line)
		case isFav:
			line = v.Styles.Favorite.Render(line)
		}
		lines[i] = line
	}
	return styledHeading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
// When the filter is active or committed it shows the query; otherwise it
// returns a plain blank line.
func (v View) filterSeparator() string {
	return v.buildFilterSeparator(true)
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	return v.buildFilterSeparator(false)
}

// buildFilterSeparator builds the filter/sort separator string. When
// leadingNewline is true, sort and filter lines are prefixed with a newline
// (used below a heading); when false the leading newline is omitted (used
// at the top level where the title bar already provides spacing).
func (v View) buildFilterSeparator(leadingNewline bool) string {
	var prefix string
	if v.SortIndicator != "" {
		if leadingNewline {
			prefix = "\n" + v.SortIndicator
		} else {
			prefix = v.SortIndicator + "\n"
		}
	}
	if v.FilterActive {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		if v.FilterText == "" {
			return prefix + sep + "Filter: > \n"
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: > ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
		}
		return prefix + sep + "Filter: > " + ft + "\n"
	}
	if v.FilterText != "" {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && len([]rune(ft)) > maxLen {
			ft = string([]rune(ft)[:maxLen-1]) + "…"
		}
		return prefix + sep + "Filter: " + ft + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}

// FormatDuration converts seconds (float64) to a "m:ss" or "h:mm:ss" string.
func FormatDuration(seconds float64) string {
	ms := int64(seconds * 1000)
	if ms <= 0 {
		return "--:--"
	}
	totalSec := ms / 1000
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Truncate shortens s to at most maxLen runes, appending "..." when truncated.
// Returns an empty string when maxLen is zero or negative.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
