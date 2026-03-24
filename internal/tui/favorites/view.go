package favorites

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the favorites panel.
type Level int

const (
	CategoryLevel Level = iota // showing the three category rows
	ListLevel                  // showing favorites in selected category
	TrackLevel                 // showing tracks for a favorite artist/album drill-down
)

// Category identifies which type of favorite is selected.
type Category int

const (
	CategoryArtists Category = iota
	CategoryAlbums
	CategoryTracks
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the favorites panel with 3-level drill-down:
// Categories -> List -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Category Category

	// Category counts (shown at CategoryLevel).
	ArtistCount int
	AlbumCount  int
	TrackCount  int

	// List items (populated at ListLevel).
	Artists []string
	Albums  []string
	Tracks  []library.Track

	// Drill-down tracks (populated at TrackLevel from a favorite artist/album).
	DrilldownTracks []library.Track

	// Saved cursors/scroll for each level (restored on Back).
	CategoryCursor   int
	ListCursor       int
	ListScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string

	SortIndicator string

	// SelectedItems tracks bulk-selected items at ListLevel/TrackLevel.
	SelectedItems  map[int]struct{}
	FavoriteHashes map[string]struct{}
}

// EffectiveItems returns the list items filtered by FilterText for the current category.
func (v View) EffectiveItems() []string {
	switch v.Category {
	case CategoryArtists:
		return arttui.FilterStrings(v.Artists, v.FilterText)
	case CategoryAlbums:
		return arttui.FilterStrings(v.Albums, v.FilterText)
	default:
		return nil
	}
}

// EffectiveTracks returns the track list filtered by FilterText.
// At TrackLevel with drill-down it uses DrilldownTracks; at ListLevel with
// CategoryTracks it uses Tracks.
func (v View) EffectiveTracks() []library.Track {
	tracks := v.currentTracks()
	return arttui.FilterTracks(tracks, v.FilterText)
}

// currentTracks returns the appropriate track slice for the current level/category.
func (v View) currentTracks() []library.Track {
	if v.Level == TrackLevel {
		return v.DrilldownTracks
	}
	return v.Tracks
}

// visibleRows returns the maximum number of rows that fit on screen.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case CategoryLevel:
		// No chrome needed for the 3-row category view.
	case ListLevel:
		if v.Category == CategoryTracks {
			reserve = 1 // column header row
		}
		if v.FilterActive || v.FilterText != "" {
			reserve++
		}
	default:
		reserve = 3 // heading + separator + column header row
	}
	if v.SortIndicator != "" {
		reserve++
	}
	return layout.VisibleRows(v.Height, reserve)
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	switch v.Level {
	case CategoryLevel:
		return 3
	case ListLevel:
		if v.Category == CategoryTracks {
			return len(v.EffectiveTracks())
		}
		return len(v.EffectiveItems())
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return 0
	}
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

// OpenList transitions from CategoryLevel to ListLevel for the selected category.
func (v View) OpenList() View {
	v.CategoryCursor = v.Cursor
	v.Category = Category(v.Cursor)
	v.Level = ListLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// OpenTracks transitions from ListLevel to TrackLevel with the given tracks.
func (v View) OpenTracks(tracks []library.Track) View {
	v.ListCursor = v.Cursor
	v.ListScrollOffset = v.ScrollOffset
	v.DrilldownTracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	v.SelectedItems = nil
	return v
}

// Back navigates one level up.
func (v View) Back() View {
	switch v.Level {
	case ListLevel:
		v.Level = CategoryLevel
		v.Cursor = v.CategoryCursor
		v.ScrollOffset = 0
		v.Artists = nil
		v.Albums = nil
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v.SelectedItems = nil
	case TrackLevel:
		v.Level = ListLevel
		v.Cursor = v.ListCursor
		v.ScrollOffset = v.ListScrollOffset
		v.DrilldownTracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v.SelectedItems = nil
	}
	return v
}

// ---------- Rendering ----------

// Render returns the string representation of the favorites panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderCategories()
	}
}

func (v View) renderCategories() string {
	categories := []string{
		fmt.Sprintf("Artists (%d)", v.ArtistCount),
		fmt.Sprintf("Albums (%d)", v.AlbumCount),
		fmt.Sprintf("Tracks (%d)", v.TrackCount),
	}

	lines := make([]string, len(categories))
	for i, name := range categories {
		display := arttui.Truncate(name, v.Width)
		display = fmt.Sprintf("%-*s", v.Width, display)
		if i == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return strings.Join(lines, "\n")
}

func (v View) renderList() string {
	if v.Category == CategoryTracks {
		return v.renderTrackList()
	}

	var categoryName string
	switch v.Category {
	case CategoryArtists:
		categoryName = "Favorite Artists"
	case CategoryAlbums:
		categoryName = "Favorite Albums"
	}
	heading := v.Styles.Heading.Render(categoryName)

	items := v.EffectiveItems()
	var allItems []string
	switch v.Category {
	case CategoryArtists:
		allItems = v.Artists
	case CategoryAlbums:
		allItems = v.Albums
	}
	if len(allItems) == 0 {
		return heading + "\n\n(empty)"
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
		isFocused := absIdx == v.Cursor

		var marker string
		_, selected := v.SelectedItems[absIdx]
		switch {
		case selected && isFocused:
			marker = "● "
		case selected:
			marker = v.Styles.AccentMark.Render("●") + " "
		default:
			marker = "  "
		}

		display := arttui.Truncate(name, v.Width-2)
		display = fmt.Sprintf("%-*s", v.Width-2, display)
		line := marker + display
		if isFocused {
			line = v.Styles.Accent.Render(line)
		}
		lines[i] = line
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTrackList() string {
	heading := v.Styles.Heading.Render("Favorite Tracks")
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(empty)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTracks() string {
	var headingText string
	switch v.Category {
	case CategoryArtists:
		items := v.EffectiveItems()
		if v.ListCursor >= 0 && v.ListCursor < len(items) {
			headingText = items[v.ListCursor]
		} else if v.ListCursor >= 0 && v.ListCursor < len(v.Artists) {
			headingText = v.Artists[v.ListCursor]
		}
	case CategoryAlbums:
		items := v.EffectiveItems()
		if v.ListCursor >= 0 && v.ListCursor < len(items) {
			headingText = items[v.ListCursor]
		} else if v.ListCursor >= 0 && v.ListCursor < len(v.Albums) {
			headingText = v.Albums[v.ListCursor]
		}
	}
	heading := v.Styles.Heading.Render(headingText)

	items := v.EffectiveTracks()
	if len(v.DrilldownTracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTrackRows(items []library.Track) string {
	cols := ComputeTrack(v.Width)

	// Column header row.
	header := v.Styles.ColHead.Render(
		fmt.Sprintf("  %-*s %-*s %-*s %-*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Artist, "Artist", cols.Album, "Album", cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor

		var marker string
		_, selected := v.SelectedItems[absIdx]
		switch {
		case selected && isFocused:
			marker = "● "
		case selected:
			marker = v.Styles.AccentMark.Render("●") + " "
		default:
			marker = "  "
		}

		pos := fmt.Sprintf("%-*d", cols.Position, absIdx+1)
		title := arttui.Pad(t.Title, cols.Title)
		artist := arttui.Pad(t.Artist, cols.Artist)
		album := arttui.Pad(t.Album, cols.Album)
		dur := arttui.FormatDuration(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		line := marker + pos + " " + title + " " + artist + " " + album + " " + dur
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
	return header + "\n" + strings.Join(lines, "\n")
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
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return sortLine + "\nFilter: > " + ft + "\n"
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
