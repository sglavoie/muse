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
type PlaylistsView struct {
	width  int
	height int
}

func (v PlaylistsView) render() string {
	// TODO: use v.width and v.height to compute proportional column widths when
	// the Playlists view is fully implemented (req-019ce36041057004a43d4d90f4a13f40).
	// The current placeholder ignores terminal dimensions; the resize requirement
	// will not be satisfied for this panel until the real render is written.
	return "Playlists"
}

// QueueEntry holds a single item in the playback queue.
type QueueEntry struct {
	Path     string // file path used to identify the entry
	Display  string // human-readable label, e.g. "Artist – Title"
	Missing  bool   // true when the file was not found at playback time
	Artist   string // track artist, used by the status bar persistent left zone
	Title    string // track title, used by the status bar persistent left zone
	Selected bool   // true when the entry is part of a bulk selection
}

// QueueView renders the playback queue panel.
//
// Key-input routing: all queue actions are dispatched by the root model via
// HandleAction after the keybinding registry resolves the raw key to an action.
// QueueView.Update is therefore a no-op stub kept only so the type satisfies a
// (QueueView, tea.Cmd) call-site contract. Do NOT add key-string handlers to
// Update — that would duplicate registry logic and create a path where registry
// key renames silently stop working.
type QueueView struct {
	entries []QueueEntry
	cursor  int // index of the currently selected entry
	width   int
	height  int
}

// Update is a no-op stub. See the QueueView godoc for why all key handling is
// done via HandleAction instead.
func (v QueueView) Update(_ tea.Msg) (QueueView, tea.Cmd) {
	return v, nil
}

// HandleAction executes a pre-resolved queue action dispatched by root.Update.
// Root resolves the raw key to an action via the keybinding registry and calls
// this method, keeping key-to-action mapping in one authoritative location.
func (v QueueView) HandleAction(a action) (QueueView, tea.Cmd) {
	switch a {
	case actionCursorUp:
		if v.cursor > 0 {
			v.cursor--
		}
	case actionCursorDown:
		if v.cursor < len(v.entries)-1 {
			v.cursor++
		}
	case actionSelect:
		if len(v.entries) > 0 && !v.entries[v.cursor].Missing {
			idx := v.cursor
			return v, func() tea.Msg { return PlayQueueIndexMsg{Index: idx} }
		}
	case actionDequeue:
		if len(v.entries) > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case actionBulkSelectionToggle:
		if len(v.entries) > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: idx} }
		}
	case actionClearQueue:
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case actionMoveTrackUp:
		if v.cursor > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
		}
	case actionMoveTrackDown:
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
	const cursorWidth = 2 // "> " or "  "
	lines := make([]string, len(v.entries))
	for i, e := range v.entries {
		cursor := "  "
		if i == v.cursor {
			cursor = "> "
		}
		display := e.Display
		if v.width > 0 {
			display = truncateTitle(display, v.width-cursorWidth)
		}
		if e.Missing {
			lines[i] = fmt.Sprintf("%s%s%s [missing]%s", cursor, ansiDim, display, ansiReset)
		} else {
			lines[i] = cursor + display
		}
	}
	return "Queue\n\n" + strings.Join(lines, "\n")
}
