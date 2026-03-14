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
	ID         int64  // database row ID; 0 when not persisted
	Path       string // file path used to identify the entry
	Hash       string // BLAKE3 content hash; used to rebuild player.QueueTrack on undo
	Display    string // human-readable label, e.g. "Artist – Title"
	Missing    bool   // true when the file was not found at playback time
	Artist     string // track artist, used by the status bar persistent left zone
	Title      string // track title, used by the status bar persistent left zone
	Selected   bool   // true when the entry is part of a bulk selection
	DurationMs int64  // track duration in milliseconds from the database
	Position   int64  // gapped-integer queue position from the database
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

// render builds the queue panel string using a 4-column layout:
//
//	Position (~4 chars fixed) | Title (~40% of width) | Artist (~30% of width) | Duration (~10 chars fixed)
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v QueueView) render() string {
	if len(v.entries) == 0 {
		return "Queue\n\n(empty)"
	}

	const (
		posWidth = 4  // "> 1" .. ">999" or "   1".."  999"
		durWidth = 10 // "59:59     " or "--:--     "
	)

	titleWidth := 20 // sensible fallback when width is unknown
	artistWidth := 15
	if v.width > 0 {
		titleWidth = v.width * 40 / 100
		artistWidth = v.width * 30 / 100
	}
	if titleWidth < 1 {
		titleWidth = 1
	}
	if artistWidth < 1 {
		artistWidth = 1
	}

	lines := make([]string, len(v.entries))
	for i, e := range v.entries {
		// Position column: cursor indicator (" " or ">") + right-aligned index.
		cursorChar := " "
		if i == v.cursor {
			cursorChar = ">"
		}
		pos := fmt.Sprintf("%s%3d", cursorChar, i+1)

		title := truncateTitle(e.Title, titleWidth)
		title = fmt.Sprintf("%-*s", titleWidth, title)

		artist := truncateTitle(e.Artist, artistWidth)
		artist = fmt.Sprintf("%-*s", artistWidth, artist)

		dur := fmt.Sprintf("%-*s", durWidth, formatQueueDuration(e.DurationMs))

		line := pos + " " + title + " " + artist + " " + dur
		if e.Missing {
			lines[i] = ansiDim + line + " [missing]" + ansiReset
		} else {
			lines[i] = line
		}
	}
	return "Queue\n\n" + strings.Join(lines, "\n")
}

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
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
