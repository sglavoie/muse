package playlists

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
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
	CreateInput      TextInput          // current value of the inline create field
	Renaming         bool               // true while the inline rename input is active
	RenameInput      TextInput          // current value of the inline rename field
	SelectedTrackIDs map[int64]struct{} // bulk-selected playlist track row IDs
	Styles           styles.Styles
}

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve 1 line for the header.
func (v View) visibleRows() int {
	return layout.VisibleRows(v.Height, 1)
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

// StartCreate returns a copy of v with the inline create input activated and
// its buffer cleared.
func (v View) StartCreate() View {
	v.Creating = true
	v.CreateInput = NewTextInput("")
	return v
}

// CancelCreate returns a copy of v with the inline create input dismissed.
func (v View) CancelCreate() View {
	v.Creating = false
	v.CreateInput = TextInput{}
	return v
}

// AppendCreateChar returns a copy of v with r appended to the create buffer.
func (v View) AppendCreateChar(r rune) View {
	v.CreateInput = v.CreateInput.AppendRune(r)
	return v
}

// BackspaceCreate returns a copy of v with the last rune removed from the
// create buffer.
func (v View) BackspaceCreate() View {
	v.CreateInput = v.CreateInput.Backspace()
	return v
}

// StartRename returns a copy of v with the inline rename input activated,
// pre-populated with currentName.
func (v View) StartRename(currentName string) View {
	v.Renaming = true
	v.RenameInput = NewTextInput(currentName)
	return v
}

// CancelRename returns a copy of v with the inline rename input dismissed.
func (v View) CancelRename() View {
	v.Renaming = false
	v.RenameInput = TextInput{}
	return v
}

// AppendRenameChar returns a copy of v with r appended to the rename buffer.
func (v View) AppendRenameChar(r rune) View {
	v.RenameInput = v.RenameInput.AppendRune(r)
	return v
}

// BackspaceRename returns a copy of v with the last rune removed from the
// rename buffer.
func (v View) BackspaceRename() View {
	v.RenameInput = v.RenameInput.Backspace()
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
		return "(no playlists)"
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
		name := truncate(p.Name, cols.Name)
		name = fmt.Sprintf("%-*s", cols.Name, name)
		count := fmt.Sprintf("(%d)", p.TrackCount)
		count = fmt.Sprintf("%*s", cols.Count, count)
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
	return strings.Join(lines, "\n")
}

// renderCreateInputRow renders the inline text input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderCreateInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := truncate(v.CreateInput.Render(), rowWidth)
	display = fmt.Sprintf("%-*s", rowWidth, display)
	return v.Styles.Accent.Render(display)
}

// renderRenameInputRow renders the inline rename input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderRenameInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := truncate(v.RenameInput.Render(), rowWidth)
	display = fmt.Sprintf("%-*s", rowWidth, display)
	return v.Styles.Accent.Render(display)
}

func (v View) renderTracks() string {
	if len(v.Tracks) == 0 {
		return v.Styles.Heading.Render("Tracks") + "\n\n(no tracks)"
	}

	cols := ComputeTrack(v.Width)

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
		title := truncate(t.Title, cols.Title)
		title = fmt.Sprintf("%-*s", cols.Title, title)
		artist := truncate(t.Artist, cols.Artist)
		artist = fmt.Sprintf("%-*s", cols.Artist, artist)
		dur := formatDuration(t.DurationMs)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		line := markerStr + pos + " " + title + " " + artist + " " + dur
		if isFocused {
			line = v.Styles.Accent.Render(line)
		}
		lines[i] = line
	}
	return v.Styles.Heading.Render("Tracks") + "\n\n" + strings.Join(lines, "\n")
}

// formatDuration converts milliseconds to a "m:ss" or "h:mm:ss" string.
func formatDuration(ms int64) string {
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

// truncate shortens s to at most maxLen runes, appending "…" when truncated.
// Returns an empty string when maxLen is zero or negative.
func truncate(s string, maxLen int) string {
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
