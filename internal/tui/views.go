package tui

import (
	"fmt"
	"strings"
)

// viewID identifies which primary view is active.
type viewID int

const (
	viewLibrary   viewID = 1
	viewPlaylists viewID = 2
	viewQueue     viewID = 3
)

// ansiDim / ansiReset wrap text in the ANSI dim (faint) attribute used to
// display missing queue entries in a grayed-out style.
const (
	ansiDim   = "\033[2m"
	ansiReset = "\033[0m"
)

// PlaylistsView is a placeholder for the playlists panel.
type PlaylistsView struct{}

func (v PlaylistsView) render() string { return "Playlists" }

// QueueEntry holds a single item in the playback queue.
type QueueEntry struct {
	Path    string // file path used to identify the entry
	Display string // human-readable label, e.g. "Artist – Title"
	Missing bool   // true when the file was not found at playback time
}

// QueueView renders the playback queue panel.
type QueueView struct {
	entries []QueueEntry
}

// markMissing marks the first entry whose Path matches as missing.
func (v *QueueView) markMissing(path string) {
	for i := range v.entries {
		if v.entries[i].Path == path {
			v.entries[i].Missing = true
			return
		}
	}
}

// render builds the queue panel string. Missing entries are wrapped in ANSI
// dim escape codes so they appear grayed out in the terminal.
func (v QueueView) render() string {
	if len(v.entries) == 0 {
		return "Queue\n\n(empty)"
	}
	lines := make([]string, len(v.entries))
	for i, e := range v.entries {
		if e.Missing {
			lines[i] = fmt.Sprintf("%s%s [missing]%s", ansiDim, e.Display, ansiReset)
		} else {
			lines[i] = e.Display
		}
	}
	return "Queue\n\n" + strings.Join(lines, "\n")
}
