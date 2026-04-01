package playlists

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Level identifies the active navigation level inside the playlist panel.
type Level int

const (
	ListLevel  Level = iota // showing the flat list of playlists
	TrackLevel              // showing the tracks inside a selected playlist
)

// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the playlists panel. All fields are set
// by the caller (PlaylistsView in the tui package) before each Render call.
type View struct {
	Level            Level
	Cursor           int
	ListCursor       int // saved list cursor; restored when BackToList is called
	ScrollOffset     int // first visible row index
	Playlists        []db.PlaylistRow
	Tracks           []db.PlaylistTrackRow // populated when Level == TrackLevel
	Width            int
	Height           int
	Creating         bool               // true while the inline create input is active
	CreateInput      textinput.Model    // current value of the inline create field
	Renaming         bool               // true while the inline rename input is active
	RenameInput      textinput.Model    // current value of the inline rename field
	SelectedTrackIDs map[int64]struct{} // bulk-selected playlist track row IDs
	FavoriteHashes   map[string]struct{}
	FavoriteArtists  map[string]struct{}
	FavoriteAlbums   map[string]struct{}
	Styles           styles.Styles
	IsSmartPlaylist  bool // true when the open playlist is a smart playlist (TrackLevel)
	SortField        string
	SortDir          string
}

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve 1 line for the list level plus 1 extra when a sort indicator is
// shown, or 4 for the track level ("Tracks" heading + blank line + header
// row + 1 padding).
func (v View) visibleRows() int {
	reserve := 1
	if v.Level == ListLevel && v.SortField != "" {
		reserve++
	}
	if v.Level == TrackLevel {
		reserve = 4
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
// stopping at the last entry. The upper bound depends on the active level:
// TrackLevel uses the track slice length; ListLevel uses the playlist slice.
// The scroll offset is adjusted so the cursor stays visible.
func (v View) CursorDown() View {
	limit := len(v.Playlists) - 1
	if v.Level == TrackLevel {
		limit = len(v.Tracks) - 1
	}
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

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	if v.Level == TrackLevel {
		return len(v.Tracks)
	}
	return len(v.Playlists)
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

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks
// loaded. The current list cursor is saved to ListCursor so it can be
// restored by BackToList. The cursor is reset to 0 for track-level navigation.
func (v View) OpenTracks(tracks []db.PlaylistTrackRow) View {
	v.ListCursor = v.Cursor
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// BackToList returns a copy of v transitioned back to ListLevel, restoring
// the cursor to the position saved by the preceding OpenTracks call and
// clearing the track slice.
func (v View) BackToList() View {
	v.Level = ListLevel
	v.Cursor = v.ListCursor
	v.Tracks = nil
	return v
}

// NewInlineInput creates a focused textinput suitable for inline row editing.
func NewInlineInput(initial string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Focus()
	return ti
}

// StartCreate returns a copy of v with the inline create input activated and
// its buffer cleared.
func (v View) StartCreate() View {
	v.Creating = true
	v.CreateInput = NewInlineInput("")
	return v
}

// CancelCreate returns a copy of v with the inline create input dismissed.
func (v View) CancelCreate() View {
	v.Creating = false
	v.CreateInput = textinput.Model{}
	return v
}

// StartRename returns a copy of v with the inline rename input activated,
// pre-populated with currentName.
func (v View) StartRename(currentName string) View {
	v.Renaming = true
	v.RenameInput = NewInlineInput(currentName)
	return v
}

// CancelRename returns a copy of v with the inline rename input dismissed.
func (v View) CancelRename() View {
	v.Renaming = false
	v.RenameInput = textinput.Model{}
	return v
}

// ToggleBulkSelection toggles id in the bulk-selected set. No-op when id is 0.
// The map is lazily initialised on first toggle.
func (v View) ToggleBulkSelection(id int64) View {
	if id == 0 {
		return v
	}
	if v.SelectedTrackIDs == nil {
		v.SelectedTrackIDs = make(map[int64]struct{})
	}
	if _, ok := v.SelectedTrackIDs[id]; ok {
		delete(v.SelectedTrackIDs, id)
	} else {
		v.SelectedTrackIDs[id] = struct{}{}
	}
	return v
}

// ClearBulkSelection removes all bulk-selection markers.
func (v View) ClearBulkSelection() View {
	v.SelectedTrackIDs = nil
	return v
}

// Render returns the string representation of the playlists panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	default:
		return v.renderTracks()
	}
}

func (v View) renderList() string {
	if len(v.Playlists) == 0 && !v.Creating {
		if v.SortField == "" {
			return "(no playlists)"
		}
		return v.renderListSortIndicator() + "\n(no playlists)"
	}

	cols := Compute(v.Width)

	maxRows := v.visibleRows()
	start := v.ScrollOffset
	if start > len(v.Playlists) {
		start = len(v.Playlists)
	}
	end := min(len(v.Playlists), start+maxRows)

	var lines []string
	for _, p := range v.Playlists[start:end] {
		absIdx := start + len(lines) // track absolute index before appending
		if v.Renaming && absIdx == v.Cursor {
			lines = append(lines, v.renderRenameInputRow(cols))
			continue
		}
		var name string
		var count string
		if p.IsSmart {
			// Smart playlists show a ⚡ glyph and either the evaluated track count
			// (once known) or "(smart)" as a placeholder. TrackCount < 0 means the
			// background evaluation has not yet returned.
			name = text.Pad("⚡ "+p.Name, cols.Name)
			if p.TrackCount >= 0 {
				count = fmt.Sprintf("%*s", cols.Count, fmt.Sprintf("(%d)", p.TrackCount))
			} else {
				count = fmt.Sprintf("%*s", cols.Count, "(smart)")
			}
		} else {
			name = text.Pad(p.Name, cols.Name)
			count = fmt.Sprintf("%*s", cols.Count, fmt.Sprintf("(%d)", p.TrackCount))
		}
		line := name + "  " + count
		if absIdx == v.Cursor {
			line = v.Styles.Accent.Render(line)
		}
		lines = append(lines, line)
		if v.Creating && absIdx == v.Cursor {
			lines = append(lines, v.renderCreateInputRow(cols))
		}
	}
	// When the list is empty the input is still shown at the top.
	if v.Creating && len(v.Playlists) == 0 {
		lines = append(lines, v.renderCreateInputRow(cols))
	}
	out := strings.Join(lines, "\n")
	if v.SortField != "" {
		out = v.renderListSortIndicator() + "\n" + out
	}
	return out
}

func (v View) renderListSortIndicator() string {
	return v.Styles.Dim.Render(fmt.Sprintf("Sort: %s %s", v.SortField, v.SortDir))
}

// renderCreateInputRow renders the inline text input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderCreateInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.CreateInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

// renderRenameInputRow renders the inline rename input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderRenameInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.RenameInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render("Tracks")
	if v.IsSmartPlaylist {
		heading += "  " + v.Styles.Dim.Render("(smart playlist — read-only)")
	}
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}

	cols := ComputeTrack(v.Width)
	titleHead, artistHead, albumHead, timeHead := "Title", "Artist", "Album", "Time"
	if v.SortField != "" {
		switch v.SortField {
		case "Title":
			titleHead += v.SortDir
		case "Artist":
			artistHead += v.SortDir
		case "Album":
			albumHead += v.SortDir
		case "Duration":
			timeHead += v.SortDir
		}
	}

	header := v.Styles.ColHead.Render(
		strings.Repeat(" ", cols.Marker) +
			fmt.Sprintf("%-*s %-*s %-*s %-*s %*s",
				cols.Position, "#",
				cols.Title, titleHead,
				cols.Artist, artistHead,
				cols.Album, albumHead,
				cols.Duration, timeHead))

	maxRows := v.visibleRows()
	start := v.ScrollOffset
	if start > len(v.Tracks) {
		start = len(v.Tracks)
	}
	end := min(len(v.Tracks), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range v.Tracks[start:end] {
		absIdx := start + i
		_, isSelected := v.SelectedTrackIDs[t.ID]
		isFocused := absIdx == v.Cursor

		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer background highlight covers the full row uniformly. An
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

		pos := fmt.Sprintf("%-*d", cols.Position, absIdx+1)
		dur := text.FormatDurationMs(t.DurationMs)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)

		// Per-cell favorite lookups.
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]

		titleStr := text.Pad(t.Title, cols.Title)
		artistStr := text.Pad(t.Artist, cols.Artist)
		albumStr := text.Pad(t.Album, cols.Album)
		if isTrackFav {
			titleStr = v.Styles.Favorite.Render(titleStr)
		}
		if isArtistFav {
			artistStr = v.Styles.FavArtist.Render(artistStr)
		}
		if isAlbumFav {
			albumStr = v.Styles.FavAlbum.Render(albumStr)
		}

		line := markerStr + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + dur
		if isFocused {
			line = v.Styles.Accent.Render(line)
		}
		lines[i] = line
	}
	return heading + "\n\n" + header + "\n" + strings.Join(lines, "\n")
}
