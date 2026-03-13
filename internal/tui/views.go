package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	Artist  string // track artist, used by the status bar persistent left zone
	Title   string // track title, used by the status bar persistent left zone
}

// QueueView renders the playback queue panel.
type QueueView struct {
	entries []QueueEntry
	cursor  int // index of the currently selected entry
}

// Update handles queue-specific key messages. Up/Down move the cursor;
// Enter dispatches a PlayQueueIndexMsg for the selected entry.
func (v QueueView) Update(msg tea.Msg) (QueueView, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return v, nil
	}
	switch key.String() {
	case "up":
		if v.cursor > 0 {
			v.cursor--
		}
	case "down":
		if v.cursor < len(v.entries)-1 {
			v.cursor++
		}
	case "enter":
		if len(v.entries) > 0 && !v.entries[v.cursor].Missing {
			idx := v.cursor
			return v, func() tea.Msg { return PlayQueueIndexMsg{Index: idx} }
		}
	case "d":
		if len(v.entries) > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case "x":
		if len(v.entries) > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: idx} }
		}
	case "C":
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case "K":
		if v.cursor > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
		}
	case "J":
		if v.cursor < len(v.entries)-1 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackDownMsg{Index: idx} }
		}
	}
	return v, nil
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
		cursor := "  "
		if i == v.cursor {
			cursor = "> "
		}
		if e.Missing {
			lines[i] = fmt.Sprintf("%s%s%s [missing]%s", cursor, ansiDim, e.Display, ansiReset)
		} else {
			lines[i] = cursor + e.Display
		}
	}
	return "Queue\n\n" + strings.Join(lines, "\n")
}
