package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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

// PlaylistsView wraps the playlists sub-panel state for the root model.
// It holds the navigation level, cursor position, loaded playlist rows, and
// (at TrackLevel) the tracks of the open playlist. Rendering is delegated to
// the playlists sub-package.
type PlaylistsView struct {
	level       pltui.Level
	cursor      int
	listCursor  int                  // saved list cursor; preserved across drill-down and back
	playlists   []db.PlaylistRow
	tracks      []db.PlaylistTrackRow // populated when level == TrackLevel
	width       int
	height      int
	creating    bool            // true while the inline create input is active
	createInput pltui.TextInput // current value of the inline create field
	renaming    bool            // true while the inline rename input is active
	renameInput pltui.TextInput // current value of the inline rename field
}

func (v PlaylistsView) render() string {
	return pltui.View{
		Level:       v.level,
		Cursor:      v.cursor,
		Playlists:   v.playlists,
		Tracks:      v.tracks,
		Width:       v.width,
		Height:      v.height,
		Creating:    v.creating,
		CreateInput: v.createInput,
		Renaming:    v.renaming,
		RenameInput: v.renameInput,
	}.Render()
}

// isCreating reports whether the inline create input is active.
func (v PlaylistsView) isCreating() bool {
	return v.creating
}

// startCreate activates the inline create input with an empty buffer.
func (v PlaylistsView) startCreate() PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.StartCreate()
	v.creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// cancelCreate dismisses the inline create input without persisting.
func (v PlaylistsView) cancelCreate() PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.CancelCreate()
	v.creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// appendCreateChar appends r to the create input buffer.
func (v PlaylistsView) appendCreateChar(r rune) PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.AppendCreateChar(r)
	v.createInput = view.CreateInput
	return v
}

// backspaceCreate removes the last rune from the create input buffer.
func (v PlaylistsView) backspaceCreate() PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.BackspaceCreate()
	v.createInput = view.CreateInput
	return v
}

// createInputValue returns the current create input buffer as a string.
func (v PlaylistsView) createInputValue() string {
	return v.createInput.Value()
}

// createInputIsEmpty reports whether the trimmed create input buffer is empty.
func (v PlaylistsView) createInputIsEmpty() bool {
	return v.createInput.IsEmpty()
}

// isRenaming reports whether the inline rename input is active.
func (v PlaylistsView) isRenaming() bool {
	return v.renaming
}

// startRename activates the inline rename input pre-populated with currentName.
func (v PlaylistsView) startRename(currentName string) PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.StartRename(currentName)
	v.renaming = view.Renaming
	v.renameInput = view.RenameInput
	return v
}

// cancelRename dismisses the inline rename input without persisting.
func (v PlaylistsView) cancelRename() PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.CancelRename()
	v.renaming = view.Renaming
	v.renameInput = view.RenameInput
	return v
}

// appendRenameChar appends r to the rename input buffer.
func (v PlaylistsView) appendRenameChar(r rune) PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.AppendRenameChar(r)
	v.renameInput = view.RenameInput
	return v
}

// backspaceRename removes the last rune from the rename input buffer.
func (v PlaylistsView) backspaceRename() PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.BackspaceRename()
	v.renameInput = view.RenameInput
	return v
}

// renameInputValue returns the current rename input buffer as a string.
func (v PlaylistsView) renameInputValue() string {
	return v.renameInput.Value()
}

// renameInputIsEmpty reports whether the trimmed rename input buffer is empty.
func (v PlaylistsView) renameInputIsEmpty() bool {
	return v.renameInput.IsEmpty()
}

// focusedPlaylist returns the playlist at the current cursor position and true.
// Returns a zero value and false when the list is empty or not at ListLevel.
func (v PlaylistsView) focusedPlaylist() (db.PlaylistRow, bool) {
	if len(v.playlists) == 0 || v.level != pltui.ListLevel {
		return db.PlaylistRow{}, false
	}
	return v.playlists[v.cursor], true
}

// HandleAction executes a pre-resolved playlists action dispatched by root.Update.
func (v PlaylistsView) HandleAction(a action) PlaylistsView {
	view := pltui.View{
		Level:     v.level,
		Cursor:    v.cursor,
		Playlists: v.playlists,
		Tracks:    v.tracks,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	}
	v.level = view.Level
	v.cursor = view.Cursor
	return v
}

// focusedPlaylistID returns the ID of the playlist at the current cursor
// position and true. Returns 0, false when the list is empty or the view is
// not at ListLevel.
func (v PlaylistsView) focusedPlaylistID() (int64, bool) {
	if len(v.playlists) == 0 || v.level != pltui.ListLevel {
		return 0, false
	}
	return v.playlists[v.cursor].ID, true
}

// openTracks transitions the view to TrackLevel, storing the loaded tracks
// and resetting the cursor to the first row. The current list cursor is saved
// so it can be restored by backToList.
func (v PlaylistsView) openTracks(tracks []db.PlaylistTrackRow) PlaylistsView {
	view := pltui.View{
		Level:      v.level,
		Cursor:     v.cursor,
		ListCursor: v.listCursor,
	}.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.listCursor = view.ListCursor
	v.tracks = view.Tracks
	return v
}

// backToList transitions the view back to ListLevel, restoring the list cursor
// to the position it held before the drill-down.
func (v PlaylistsView) backToList() PlaylistsView {
	view := pltui.View{
		Level:      v.level,
		Cursor:     v.cursor,
		ListCursor: v.listCursor,
	}.BackToList()
	v.level = view.Level
	v.cursor = view.Cursor
	v.tracks = view.Tracks
	return v
}

// atListLevel reports whether the view is currently showing the playlist list.
func (v PlaylistsView) atListLevel() bool {
	return v.level == pltui.ListLevel
}

// focusedTrack returns the musiclib.Track for the cursor position at TrackLevel.
// Returns false when the view is not at TrackLevel or the track list is empty.
func (v PlaylistsView) focusedTrack() (musiclib.Track, bool) {
	if v.level != pltui.TrackLevel || len(v.tracks) == 0 {
		return musiclib.Track{}, false
	}
	if v.cursor < 0 || v.cursor >= len(v.tracks) {
		return musiclib.Track{}, false
	}
	t := v.tracks[v.cursor]
	return musiclib.Track{
		Hash:     t.Hash,
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Duration: float64(t.DurationMs) / 1000,
	}, true
}

// enqueueFocusedTrack returns a tea.Cmd that appends the focused track to the
// play queue via qa. prevQueueLen is used to set WasEmpty on the returned
// TrackEnqueuedMsg so the root model can trigger immediate playback when needed.
// Returns nil when the view is not at TrackLevel or no track is focused.
func (v PlaylistsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	if !ok {
		return nil
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		id, err := qa.AppendTrack(track)
		return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
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
