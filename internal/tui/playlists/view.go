package playlists

import (
	"fmt"
	"strings"

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
	Level     Level
	Cursor    int
	Playlists []db.PlaylistRow
	Width     int
	Height    int
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
// stopping at the last entry.
func (v View) CursorDown() View {
	if v.Cursor < len(v.Playlists)-1 {
		v.Cursor++
	}
	return v
}

// Render returns the string representation of the playlists panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	default:
		return "Playlists"
	}
}

func (v View) renderList() string {
	if len(v.Playlists) == 0 {
		return "Playlists\n\n(no playlists)"
	}

	cols := Compute(v.Width)
	lines := make([]string, len(v.Playlists))
	for i, p := range v.Playlists {
		name := truncate(p.Name, cols.Name)
		name = fmt.Sprintf("%-*s", cols.Name, name)
		count := fmt.Sprintf("(%d)", p.TrackCount)
		count = fmt.Sprintf("%*s", cols.Count, count)
		line := name + "  " + count
		if i == v.Cursor {
			line = ansiAccent + line + ansiReset
		}
		lines[i] = line
	}
	return "Playlists\n\n" + strings.Join(lines, "\n")
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
