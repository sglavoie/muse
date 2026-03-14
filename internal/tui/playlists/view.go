package playlists

import (
	"fmt"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// Level identifies the active navigation level inside the playlist panel.
type Level int

const (
	ListLevel  Level = iota // showing the flat list of playlists
	TrackLevel              // showing the tracks inside a selected playlist
)

// View is the pure render struct for the playlists panel. All fields are set
// by the caller (PlaylistsView in the tui package) before each Render call.
type View struct {
	Level       Level
	Cursor      int
	ListCursor  int                  // saved list cursor; restored when BackToList is called
	Playlists   []db.PlaylistRow
	Tracks      []db.PlaylistTrackRow // populated when Level == TrackLevel
	Width       int
	Height      int
	Creating    bool      // true while the inline create input is active
	CreateInput TextInput // current value of the inline create field
}

// CursorUp returns a copy of v with the cursor moved up by one row,
// stopping at the first entry.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row,
// stopping at the last entry. The upper bound depends on the active level:
// TrackLevel uses the track slice length; ListLevel uses the playlist slice.
func (v View) CursorDown() View {
	limit := len(v.Playlists) - 1
	if v.Level == TrackLevel {
		limit = len(v.Tracks) - 1
	}
	if v.Cursor < limit {
		v.Cursor++
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
		return "Playlists\n\n(no playlists)"
	}

	cols := Compute(v.Width)
	var lines []string
	for i, p := range v.Playlists {
		name := truncate(p.Name, cols.Name)
		name = fmt.Sprintf("%-*s", cols.Name, name)
		count := fmt.Sprintf("(%d)", p.TrackCount)
		count = fmt.Sprintf("%*s", cols.Count, count)
		line := name + "  " + count
		if i == v.Cursor {
			line = ansiAccent + line + ansiReset
		}
		lines = append(lines, line)
		if v.Creating && i == v.Cursor {
			lines = append(lines, v.renderCreateInputRow(cols))
		}
	}
	// When the list is empty the input is still shown at the top.
	if v.Creating && len(v.Playlists) == 0 {
		lines = append(lines, v.renderCreateInputRow(cols))
	}
	return "Playlists\n\n" + strings.Join(lines, "\n")
}

// renderCreateInputRow renders the inline text input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderCreateInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := truncate(v.CreateInput.Render(), rowWidth)
	display = fmt.Sprintf("%-*s", rowWidth, display)
	return ansiAccent + display + ansiReset
}

func (v View) renderTracks() string {
	if len(v.Tracks) == 0 {
		return "Tracks\n\n(no tracks)"
	}

	cols := ComputeTrack(v.Width)
	lines := make([]string, len(v.Tracks))
	for i, t := range v.Tracks {
		pos := fmt.Sprintf("%-*d", cols.Position, t.Position)
		title := truncate(t.Title, cols.Title)
		title = fmt.Sprintf("%-*s", cols.Title, title)
		artist := truncate(t.Artist, cols.Artist)
		artist = fmt.Sprintf("%-*s", cols.Artist, artist)
		dur := formatDuration(t.DurationMs)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		line := pos + " " + title + " " + artist + " " + dur
		if i == v.Cursor {
			line = ansiAccent + line + ansiReset
		}
		lines[i] = line
	}
	return "Tracks\n\n" + strings.Join(lines, "\n")
}

// formatDuration converts milliseconds to a "m:ss" string.
func formatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	totalSec := int(d.Seconds())
	minutes := totalSec / 60
	seconds := totalSec % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
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
