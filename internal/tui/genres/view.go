package genres

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Level identifies the active navigation level inside the genres panel.
type Level int

const (
	GenreLevel  Level = iota // showing the flat list of genres
	ArtistLevel              // showing artists for a selected genre
	TrackLevel               // showing tracks for a selected artist
)

const (
	UnknownGenre = "(unknown genre)"
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the genres panel with 3-level drill-down:
// Genres -> Artists -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Genres  []string
	Artists []string
	Tracks  []library.Track

	SelectedGenre  string
	SelectedArtist string

	// Saved cursors/scroll for each level (restored on Back).
	GenreCursor        int
	GenreScrollOffset  int
	ArtistCursor       int
	ArtistScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string

	SortIndicator  string // e.g. "Sort: Name ▲" — shown when non-empty
	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
}

// RawGenre converts the unknown sentinel back to an empty string for matching
// against raw track metadata. All other values pass through.
func RawGenre(s string) string {
	if s == UnknownGenre {
		return ""
	}
	return s
}

// EffectiveGenres returns the genre list filtered by FilterText.
func (v View) EffectiveGenres() []string { return arttui.FilterStrings(v.Genres, v.FilterText) }

// EffectiveArtists returns the artist list filtered by FilterText.
func (v View) EffectiveArtists() []string { return arttui.FilterStrings(v.Artists, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track { return arttui.FilterTracks(v.Tracks, v.FilterText) }

// visibleRows returns the maximum number of rows that fit on screen.
// The reserve depends on the current level:
//   - GenreLevel: 0 (or 1 if filter active)
//   - ArtistLevel: 3 (heading + separator)
//   - TrackLevel: 3 (heading + separator + column header)
//
// A sort indicator adds 1 line at any level.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case GenreLevel:
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

// ---------- Data derivation (pure functions) ----------

// DeriveGenres extracts unique genre names from all tracks. Multi-genre tags
// are split via library.SplitGenres. Tracks with no genre contribute to
// UnknownGenre. Results are sorted alphabetically with unknowns last.
func DeriveGenres(tracks []library.Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var genres []string
	for _, t := range tracks {
		gs := library.SplitGenres(t.Genre)
		if len(gs) == 0 {
			if _, ok := seen[UnknownGenre]; !ok {
				seen[UnknownGenre] = struct{}{}
				genres = append(genres, UnknownGenre)
			}
			continue
		}
		for _, g := range gs {
			if _, ok := seen[g]; ok {
				continue
			}
			seen[g] = struct{}{}
			genres = append(genres, g)
		}
	}
	sort.Slice(genres, func(i, j int) bool {
		if genres[i] == UnknownGenre {
			return false
		}
		if genres[j] == UnknownGenre {
			return true
		}
		return genres[i] < genres[j]
	})
	return genres
}

// DeriveGenreArtists returns unique artist names for tracks that belong to the
// given genre. Tracks with no artist use the arttui.UnknownArtist sentinel.
// Results are sorted alphabetically with unknowns last.
func DeriveGenreArtists(tracks []library.Track, genre string) []string {
	raw := RawGenre(genre)
	seen := make(map[string]struct{})
	var artists []string
	for _, t := range tracks {
		if !trackMatchesGenre(t, raw) {
			continue
		}
		name := t.Artist
		if name == "" {
			name = arttui.UnknownArtist
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			artists = append(artists, name)
		}
	}
	sort.Slice(artists, func(i, j int) bool {
		if artists[i] == arttui.UnknownArtist {
			return false
		}
		if artists[j] == arttui.UnknownArtist {
			return true
		}
		return artists[i] < artists[j]
	})
	return artists
}

// DeriveGenreArtistTracks returns all tracks matching both the given genre and
// artist.
func DeriveGenreArtistTracks(tracks []library.Track, genre, artist string) []library.Track {
	rawGenre := RawGenre(genre)
	rawArtist := arttui.RawName(artist)
	var result []library.Track
	for _, t := range tracks {
		if !trackMatchesGenre(t, rawGenre) {
			continue
		}
		if t.Artist == rawArtist {
			result = append(result, t)
		}
	}
	return result
}

// DeriveGenreTracks returns all tracks for a genre regardless of artist.
// Used for bulk enqueue at the genre level.
func DeriveGenreTracks(tracks []library.Track, genre string) []library.Track {
	raw := RawGenre(genre)
	var result []library.Track
	for _, t := range tracks {
		if trackMatchesGenre(t, raw) {
			result = append(result, t)
		}
	}
	return result
}

// trackMatchesGenre reports whether t belongs to the genre identified by raw
// (the result of RawGenre on the display genre string). When raw is empty the
// function matches tracks that have no recognised genre.
func trackMatchesGenre(t library.Track, raw string) bool {
	if raw == "" {
		// Unknown genre: match tracks with no recognised genre split.
		return len(library.SplitGenres(t.Genre)) == 0
	}
	return slices.Contains(library.SplitGenres(t.Genre), raw)
}

// ---------- Rendering ----------

// Render returns the string representation of the genres panel.
func (v View) Render() string {
	switch v.Level {
	case ArtistLevel:
		return v.renderArtists()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderGenres()
	}
}

func (v View) renderGenres() string {
	items := v.EffectiveGenres()
	if len(v.Genres) == 0 {
		return "(no genres)"
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
		display := text.Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		if absIdx == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderArtists() string {
	heading := v.Styles.Heading.Render(v.SelectedGenre)
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return heading + "\n\n(no artists)"
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
		_, isFav := v.FavoriteArtists[name]
		display := text.Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavArtistAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = v.Styles.FavArtist.Render(display)
		}
		lines[i] = display
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTracks() string {
	heading := v.SelectedGenre
	if v.SelectedArtist != "" {
		heading += " — " + v.SelectedArtist
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
			markerStr = "● "
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%-*d", cols.Position, absIdx+1)
		titleStr := text.Pad(t.Title, cols.Title)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		if isTrackFav {
			titleStr = v.Styles.Favorite.Render(titleStr)
		}

		line := markerStr + pos + " " + titleStr + " " + dur
		if isFocused {
			line = v.Styles.Accent.Render(line)
		}
		lines[i] = line
	}
	return styledHeading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return v.buildFilterSeparator(true)
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	return v.buildFilterSeparator(false)
}

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
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
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
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return prefix + sep + "Filter: " + ft + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}
