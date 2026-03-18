package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// viewID identifies which primary view is active.
type viewID int

const (
	viewLibrary   viewID = 1
	viewPlaylists viewID = 2
	viewQueue     viewID = 3
	viewArtists   viewID = 4
	viewAlbums    viewID = 5
)

// viewOrder defines the tab-cycling order for primary views.
var viewOrder = []viewID{viewLibrary, viewPlaylists, viewQueue, viewArtists, viewAlbums}

// viewName returns the display name for a viewID.
func viewName(id viewID) string {
	switch id {
	case viewLibrary:
		return "Library"
	case viewPlaylists:
		return "Playlists"
	case viewQueue:
		return "Queue"
	case viewArtists:
		return "Artists"
	case viewAlbums:
		return "Albums"
	default:
		return ""
	}
}

// viewToContext maps a viewID to its corresponding bindingContext.
func viewToContext(id viewID) bindingContext {
	switch id {
	case viewLibrary:
		return contextLibrary
	case viewPlaylists:
		return contextPlaylists
	case viewQueue:
		return contextQueue
	case viewArtists:
		return contextArtists
	case viewAlbums:
		return contextAlbums
	default:
		return contextGlobal
	}
}

// prevView returns the previous view in the cycle (wrapping around).
func prevView(id viewID) viewID {
	for i, v := range viewOrder {
		if v == id {
			return viewOrder[(i-1+len(viewOrder))%len(viewOrder)]
		}
	}
	return viewOrder[0]
}

// nextView returns the next view in the cycle (wrapping around).
func nextView(id viewID) viewID {
	for i, v := range viewOrder {
		if v == id {
			return viewOrder[(i+1)%len(viewOrder)]
		}
	}
	return viewOrder[0]
}

// PlaylistsView wraps the playlists sub-panel state for the root model.
// It holds the navigation level, cursor position, loaded playlist rows, and
// (at TrackLevel) the tracks of the open playlist. Rendering is delegated to
// the playlists sub-package.
type PlaylistsView struct {
	level            pltui.Level
	cursor           int
	listCursor       int // saved list cursor; preserved across drill-down and back
	listScrollOffset int // saved list scroll offset; preserved across drill-down and back
	scrollOffset     int // first visible row index
	playlists        []db.PlaylistRow
	tracks           []db.PlaylistTrackRow // populated when level == TrackLevel
	width            int
	height           int
	creating         bool               // true while the inline create input is active
	createInput      textinput.Model    // current value of the inline create field
	renaming         bool               // true while the inline rename input is active
	renameInput      textinput.Model    // current value of the inline rename field
	selectedTrackIDs map[int64]struct{} // bulk-selected playlist track row IDs at TrackLevel
}

func (v PlaylistsView) render(s styles.Styles) string {
	pv := pltui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Playlists:        v.playlists,
		Tracks:           v.tracks,
		Width:            v.width,
		Height:           v.height,
		Creating:         v.creating,
		CreateInput:      v.createInput,
		Renaming:         v.renaming,
		RenameInput:      v.renameInput,
		SelectedTrackIDs: v.selectedTrackIDs,
	}
	pv.Styles = s
	return pv.Render()
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

// updateCreateInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) updateCreateInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// createInputValue returns the current create input buffer as a string.
func (v PlaylistsView) createInputValue() string {
	return v.createInput.Value()
}

// createInputIsEmpty reports whether the trimmed create input buffer is empty.
func (v PlaylistsView) createInputIsEmpty() bool {
	return strings.TrimSpace(v.createInput.Value()) == ""
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

// updateRenameInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) updateRenameInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.renameInput, cmd = v.renameInput.Update(msg)
	return v, cmd
}

// renameInputValue returns the current rename input buffer as a string.
func (v PlaylistsView) renameInputValue() string {
	return v.renameInput.Value()
}

// renameInputIsEmpty reports whether the trimmed rename input buffer is empty.
func (v PlaylistsView) renameInputIsEmpty() bool {
	return strings.TrimSpace(v.renameInput.Value()) == ""
}

// focusedPlaylist returns the playlist at the current cursor position and true.
// Returns a zero value and false when the list is empty or not at ListLevel.
func (v PlaylistsView) focusedPlaylist() (db.PlaylistRow, bool) {
	if len(v.playlists) == 0 || v.level != pltui.ListLevel {
		return db.PlaylistRow{}, false
	}
	if v.cursor < 0 || v.cursor >= len(v.playlists) {
		return db.PlaylistRow{}, false
	}
	return v.playlists[v.cursor], true
}

// HandleAction executes a pre-resolved playlists action dispatched by root.Update.
func (v PlaylistsView) HandleAction(a action) PlaylistsView {
	switch a {
	case actionBulkSelectionToggle:
		if v.level == pltui.TrackLevel && len(v.tracks) > 0 && v.cursor < len(v.tracks) {
			id := v.tracks[v.cursor].ID
			view := pltui.View{SelectedTrackIDs: v.selectedTrackIDs}.ToggleBulkSelection(id)
			v.selectedTrackIDs = view.SelectedTrackIDs
		}
		return v
	}
	view := pltui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
		Height:       v.height,
		Playlists:    v.playlists,
		Tracks:       v.tracks,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	case actionHalfPageDown:
		view = view.HalfPageDown()
	case actionHalfPageUp:
		view = view.HalfPageUp()
	case actionFullPageDown:
		view = view.FullPageDown()
	case actionFullPageUp:
		view = view.FullPageUp()
	case actionGoToTop:
		view = view.GoToTop()
	case actionGoToBottom:
		view = view.GoToBottom()
	}
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// hasTrackSelection reports whether any tracks are bulk-selected.
func (v PlaylistsView) hasTrackSelection() bool {
	return len(v.selectedTrackIDs) > 0
}

// clearTrackSelection removes all bulk-selection markers.
func (v PlaylistsView) clearTrackSelection() PlaylistsView {
	v.selectedTrackIDs = nil
	return v
}

// ensureCursorVisible adjusts scrollOffset so the cursor row is within the
// visible window. Call after operations that reposition the cursor (e.g.
// playlist rename or create) where the new cursor position may fall outside
// the current viewport.
func (v PlaylistsView) ensureCursorVisible() PlaylistsView {
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	} else {
		reserve := 3
		maxRows := 50
		if v.height > 0 {
			maxRows = max(1, v.height-reserve)
		}
		if v.cursor >= v.scrollOffset+maxRows {
			v.scrollOffset = v.cursor - maxRows + 1
		}
	}
	return v
}

// focusedPlaylistID returns the ID of the playlist at the current cursor
// position and true. Returns 0, false when the list is empty or the view is
// not at ListLevel.
func (v PlaylistsView) focusedPlaylistID() (int64, bool) {
	if len(v.playlists) == 0 || v.level != pltui.ListLevel {
		return 0, false
	}
	if v.cursor < 0 || v.cursor >= len(v.playlists) {
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
	v.listScrollOffset = v.scrollOffset // preserve list-level scroll position
	v.tracks = view.Tracks
	v.scrollOffset = 0
	v.selectedTrackIDs = nil // clear stale track-level selections from a previous playlist
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
	v.scrollOffset = v.listScrollOffset // restore list-level scroll position
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
// The source playlist name is carried through to the DB and TrackEnqueuedMsg.
func (v PlaylistsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	if !ok {
		return nil
	}
	// Determine the source playlist name from the saved list cursor.
	var playlistName string
	if v.listCursor >= 0 && v.listCursor < len(v.playlists) {
		playlistName = v.playlists[v.listCursor].Name
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		id, err := qa.AppendTrack(track, playlistName)
		return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, SourcePlaylist: playlistName}
	}
}

// QueueEntry holds a single item in the playback queue.
type QueueEntry struct {
	ID             int64  // database row ID; 0 when not persisted
	Path           string // file path used to identify the entry
	Hash           string // BLAKE3 content hash; used to rebuild player.QueueTrack on undo
	Display        string // human-readable label, e.g. "Artist – Title"
	Missing        bool   // true when the file was not found at playback time
	Artist         string // track artist, used by the status bar persistent left zone
	Title          string // track title, used by the status bar persistent left zone
	Selected       bool   // true when the entry is part of a bulk selection
	DurationMs     int64  // track duration in milliseconds from the database
	Position       int64  // gapped-integer queue position from the database
	SourcePlaylist string // playlist name this entry was enqueued from; empty for individual enqueues
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
	entries             []QueueEntry
	cursor              int // index of the currently selected entry
	scrollOffset        int // first visible entry index
	width               int
	height              int
	savingAsPlaylist    bool            // true while the inline save-as-playlist input is active
	savingFromSelection bool            // true when saving only bulk-selected entries (not the full queue)
	saveInput           textinput.Model // current value of the inline save field
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
		if v.cursor < v.scrollOffset {
			v.scrollOffset = v.cursor
		}
	case actionCursorDown:
		if v.cursor < len(v.entries)-1 {
			v.cursor++
		}
		maxRows := v.visibleRows()
		if v.cursor >= v.scrollOffset+maxRows {
			v.scrollOffset = v.cursor - maxRows + 1
		}
	case actionHalfPageDown:
		v = v.moveQueueCursor(v.visibleRows() / 2)
	case actionHalfPageUp:
		v = v.moveQueueCursor(-v.visibleRows() / 2)
	case actionFullPageDown:
		v = v.moveQueueCursor(v.visibleRows())
	case actionFullPageUp:
		v = v.moveQueueCursor(-v.visibleRows())
	case actionGoToTop:
		v.cursor = 0
		v.scrollOffset = 0
	case actionGoToBottom:
		if n := len(v.entries); n > 0 {
			v.cursor = n - 1
			maxRows := v.visibleRows()
			if v.cursor >= maxRows {
				v.scrollOffset = v.cursor - maxRows + 1
			}
		}
	case actionSelect:
		if v.cursor >= 0 && v.cursor < len(v.entries) && !v.entries[v.cursor].Missing {
			idx := v.cursor
			return v, func() tea.Msg { return PlayQueueIndexMsg{Index: idx} }
		}
	case actionDequeue:
		if v.cursor >= 0 && v.cursor < len(v.entries) {
			idx := v.cursor
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case actionBulkSelectionToggle:
		if v.cursor >= 0 && v.cursor < len(v.entries) {
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

// markMissing marks all entries whose Path matches as missing.
func (v *QueueView) markMissing(path string) {
	for i := range v.entries {
		if v.entries[i].Path == path {
			v.entries[i].Missing = true
		}
	}
}

// isSavingAsPlaylist reports whether the inline save-as-playlist input is active.
func (v QueueView) isSavingAsPlaylist() bool {
	return v.savingAsPlaylist
}

// startSaveAsPlaylist activates the inline save-as-playlist input with an empty buffer.
// When fromSelection is true the prompt says "Save selection as playlist" and
// only bulk-selected entries will be persisted.
func (v QueueView) startSaveAsPlaylist(fromSelection bool) QueueView {
	v.savingAsPlaylist = true
	v.savingFromSelection = fromSelection
	v.saveInput = pltui.NewInlineInput("")
	return v
}

// cancelSaveAsPlaylist dismisses the inline save-as-playlist input without persisting.
func (v QueueView) cancelSaveAsPlaylist() QueueView {
	v.savingAsPlaylist = false
	v.savingFromSelection = false
	v.saveInput = textinput.Model{}
	return v
}

// hasSelection reports whether any queue entries are bulk-selected.
func (v QueueView) hasSelection() bool {
	for _, e := range v.entries {
		if e.Selected {
			return true
		}
	}
	return false
}

// selectedHashes returns the hashes of all bulk-selected entries, preserving queue order.
func (v QueueView) selectedHashes() []string {
	var hashes []string
	for _, e := range v.entries {
		if e.Selected {
			hashes = append(hashes, e.Hash)
		}
	}
	return hashes
}

// updateSaveInput forwards a key message to the Bubbles textinput model.
func (v QueueView) updateSaveInput(msg tea.KeyPressMsg) (QueueView, tea.Cmd) {
	var cmd tea.Cmd
	v.saveInput, cmd = v.saveInput.Update(msg)
	return v, cmd
}

// saveInputValue returns the current save input buffer as a string.
func (v QueueView) saveInputValue() string {
	return v.saveInput.Value()
}

// saveInputIsEmpty reports whether the trimmed save input buffer is empty.
func (v QueueView) saveInputIsEmpty() bool {
	return strings.TrimSpace(v.saveInput.Value()) == ""
}

// moveQueueCursor shifts the cursor by delta rows, clamping to [0, len-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v QueueView) moveQueueCursor(delta int) QueueView {
	n := len(v.entries)
	if n == 0 {
		return v
	}
	v.cursor += delta
	v.cursor = max(0, min(v.cursor, n-1))
	maxRows := v.visibleRows()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	} else if v.cursor >= v.scrollOffset+maxRows {
		v.scrollOffset = v.cursor - maxRows + 1
	}
	return v
}

// visibleRows returns the maximum number of queue entries that fit on screen.
// Reserve 2 lines for the column header and bottom margin.
func (v QueueView) visibleRows() int {
	return layout.VisibleRows(v.height, 2)
}

// render builds the queue panel string using a 5-column layout:
//
//	Position (~4 chars fixed) | Title (~35% of width) | Artist (~25% of width) | Source (~15% of width) | Duration (~10 chars fixed)
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v QueueView) render(s styles.Styles, currentTrackHash string) string {
	if len(v.entries) == 0 {
		return "(empty)"
	}

	const (
		posWidth = 4  // "> 1" .. ">999" or "   1".."  999"
		durWidth = 10 // "59:59     " or "--:--     "
	)

	titleWidth := 20 // sensible fallback when width is unknown
	artistWidth := 15
	sourceWidth := 10
	if v.width > 0 {
		titleWidth = v.width * 35 / 100
		artistWidth = v.width * 25 / 100
		sourceWidth = v.width * 15 / 100
	}
	if titleWidth < 1 {
		titleWidth = 1
	}
	if artistWidth < 1 {
		artistWidth = 1
	}
	sourceWidth = max(sourceWidth, 1)

	maxRows := v.visibleRows()
	start := min(v.scrollOffset, len(v.entries))
	end := min(len(v.entries), start+maxRows)

	// Column header row (2-char left margin aligns with the selection marker column).
	header := fmt.Sprintf("  %3s %-*s %-*s %-*s %-*s",
		"#", titleWidth, "Title", artistWidth, "Artist", sourceWidth, "Source", durWidth, "Time")

	lines := make([]string, end-start)
	for i, e := range v.entries[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.cursor

		isNowPlaying := currentTrackHash != "" && e.Hash == currentTrackHash

		// Selection marker: matches Library and Playlists style.
		// Now-playing track gets a "▶" marker when not bulk-selected.
		var markerStr string
		switch {
		case e.Selected && isFocused:
			markerStr = "● "
		case e.Selected:
			markerStr = s.AccentMark.Render("●") + " "
		case isNowPlaying && isFocused:
			markerStr = "▶ "
		case isNowPlaying:
			markerStr = s.Success.Render("▶") + " "
		default:
			markerStr = "  "
		}

		// Position column: cursor indicator (" " or ">") + right-aligned index.
		cursorChar := " "
		if isFocused {
			cursorChar = ">"
		}
		pos := fmt.Sprintf("%s%3d", cursorChar, absIdx+1)

		title := truncateTitle(e.Title, titleWidth)
		title = fmt.Sprintf("%-*s", titleWidth, title)

		artist := truncateTitle(e.Artist, artistWidth)
		artist = fmt.Sprintf("%-*s", artistWidth, artist)

		source := truncateTitle(e.SourcePlaylist, sourceWidth)
		source = fmt.Sprintf("%-*s", sourceWidth, source)

		dur := fmt.Sprintf("%-*s", durWidth, formatQueueDuration(e.DurationMs))

		line := markerStr + pos + " " + title + " " + artist + " " + source + " " + dur
		if e.Missing {
			lines[i] = s.Dim.Render(line + " [missing]")
		} else if isFocused {
			lines[i] = s.Accent.Render(line)
		} else if isNowPlaying {
			lines[i] = s.NowPlaying.Render(line)
		} else {
			lines[i] = line
		}
	}
	out := s.ColHead.Render(header) + "\n" + strings.Join(lines, "\n")
	if v.savingAsPlaylist {
		prompt := "Save as playlist: "
		if v.savingFromSelection {
			prompt = "Save selection as playlist: "
		}
		inputLine := s.Accent.Render(prompt) + v.saveInput.View()
		out += "\n" + inputLine
	}
	return out
}

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
	return FormatDurationMs(ms)
}

// ArtistsView wraps the artists sub-panel state for the root model.
// It holds the navigation level, cursor position, artist/album/track lists,
// and saved cursors for each drill-down level. Rendering is delegated to the
// artists sub-package.
type ArtistsView struct {
	level        arttui.Level
	cursor       int
	scrollOffset int

	artists []string
	albums  []string
	tracks  []musiclib.Track

	selectedArtist string
	selectedAlbum  string

	// Saved cursors/scroll for each level (restored on back).
	artistCursor       int
	artistScrollOffset int
	albumCursor        int
	albumScrollOffset  int

	width  int
	height int

	filter filter.State
}

func (v ArtistsView) render(s styles.Styles) string {
	av := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		SelectedArtist:     v.selectedArtist,
		SelectedAlbum:      v.selectedAlbum,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Width:              v.width,
		Height:             v.height,
		Styles:             s,
		FilterActive:       v.filter.Active,
		FilterText:         v.filter.Text,
	}
	return av.Render()
}

// HandleAction executes a pre-resolved artists action dispatched by root.Update.
func (v ArtistsView) HandleAction(a action) ArtistsView {
	view := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Height:             v.height,
		FilterText:         v.filter.Text,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	case actionHalfPageDown:
		view = view.HalfPageDown()
	case actionHalfPageUp:
		view = view.HalfPageUp()
	case actionFullPageDown:
		view = view.FullPageDown()
	case actionFullPageUp:
		view = view.FullPageUp()
	case actionGoToTop:
		view = view.GoToTop()
	case actionGoToBottom:
		view = view.GoToBottom()
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// openAlbums transitions the view to AlbumLevel with the given album list.
// The current artist cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) openAlbums(albums []string) ArtistsView {
	view := arttui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenAlbums(albums)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.artistCursor = view.ArtistCursor
	v.artistScrollOffset = view.ArtistScrollOffset
	v.albums = view.Albums
	v.filter = filter.State{}
	return v
}

// openTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) openTracks(tracks []musiclib.Track) ArtistsView {
	view := arttui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.albumCursor = view.AlbumCursor
	v.albumScrollOffset = view.AlbumScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	return v
}

// back navigates one level up. At AlbumLevel it returns to ArtistLevel;
// at TrackLevel it returns to AlbumLevel (or ArtistLevel if albums were
// skipped). At ArtistLevel it is a no-op.
func (v ArtistsView) back() ArtistsView {
	view := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
	}
	view = view.Back()
	// When the album level was skipped (no albums), go straight back to artists.
	if view.Level == arttui.AlbumLevel && len(view.Albums) == 0 {
		view = view.Back()
	}
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.albums = view.Albums
	v.tracks = view.Tracks
	v.filter = filter.State{}
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// list. Call after refreshing the artist list when the library changes.
func (v ArtistsView) clampCursor() ArtistsView {
	var n int
	switch v.level {
	case arttui.AlbumLevel:
		n = len(v.albums)
	case arttui.TrackLevel:
		n = len(v.tracks)
	default:
		n = len(v.artists)
	}
	if n == 0 {
		v.cursor = 0
		v.scrollOffset = 0
	} else if v.cursor >= n {
		v.cursor = n - 1
	}
	return v
}

// ActivateFilter sets the filter bar to active input mode.
func (v ArtistsView) ActivateFilter() ArtistsView {
	v.filter = v.filter.Activate()
	return v
}

// CommitFilter exits input mode while keeping the current query visible.
func (v ArtistsView) CommitFilter() ArtistsView {
	v.filter = v.filter.Commit()
	return v
}

// DeactivateFilter clears the query, exits input mode, and resets the cursor.
func (v ArtistsView) DeactivateFilter() ArtistsView {
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	return v
}

// AppendFilterChar adds ch to the filter text and resets the cursor.
func (v ArtistsView) AppendFilterChar(ch rune) ArtistsView {
	var changed bool
	v.filter, changed = v.filter.AppendChar(ch)
	if changed {
		v.cursor = 0
		v.scrollOffset = 0
	}
	return v
}

// BackspaceFilter removes the last rune from the filter text.
func (v ArtistsView) BackspaceFilter() ArtistsView {
	var changed bool
	v.filter, changed = v.filter.Backspace()
	if changed {
		v.cursor = 0
		v.scrollOffset = 0
	}
	return v
}

// focusedArtist returns the artist name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) focusedArtist() (string, bool) {
	if v.level != arttui.ArtistLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.artists, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) focusedAlbum() (string, bool) {
	if v.level != arttui.AlbumLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.albums, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedTrack returns the track at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) focusedTrack() (musiclib.Track, bool) {
	if v.level != arttui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.tracks, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.cursor], true
}

// enqueueFocusedTrack returns a tea.Cmd that appends the focused track to the
// play queue via qa. prevQueueLen is used to set WasEmpty on the returned
// TrackEnqueuedMsg so the root model can trigger immediate playback when needed.
// Returns nil when the view is not at TrackLevel or no track is focused.
func (v ArtistsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	if !ok {
		return nil
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		id, err := qa.AppendTrack(track, "")
		return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
}

// AlbumsView wraps the albums sub-panel state for the root model.
// It holds the navigation level, cursor position, album/track lists,
// and saved cursors for drill-down. Rendering is delegated to the
// albums sub-package.
type AlbumsView struct {
	level        albtui.Level
	cursor       int
	scrollOffset int

	albums []string
	tracks []musiclib.Track

	selectedAlbum string

	// Saved cursor/scroll for album level (restored on back).
	albumCursor       int
	albumScrollOffset int

	width  int
	height int

	filter filter.State
}

func (v AlbumsView) render(s styles.Styles) string {
	av := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		SelectedAlbum:     v.selectedAlbum,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Width:             v.width,
		Height:            v.height,
		Styles:            s,
		FilterActive:      v.filter.Active,
		FilterText:        v.filter.Text,
	}
	return av.Render()
}

// HandleAction executes a pre-resolved albums action dispatched by root.Update.
func (v AlbumsView) HandleAction(a action) AlbumsView {
	view := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Height:            v.height,
		FilterText:        v.filter.Text,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	case actionHalfPageDown:
		view = view.HalfPageDown()
	case actionHalfPageUp:
		view = view.HalfPageUp()
	case actionFullPageDown:
		view = view.FullPageDown()
	case actionFullPageUp:
		view = view.FullPageUp()
	case actionGoToTop:
		view = view.GoToTop()
	case actionGoToBottom:
		view = view.GoToBottom()
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// openTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v AlbumsView) openTracks(tracks []musiclib.Track) AlbumsView {
	view := albtui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.albumCursor = view.AlbumCursor
	v.albumScrollOffset = view.AlbumScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	return v
}

// back navigates one level up. At TrackLevel it returns to AlbumLevel.
// At AlbumLevel it is a no-op.
func (v AlbumsView) back() AlbumsView {
	view := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
	}
	view = view.Back()
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// list. Call after refreshing the album list when the library changes.
func (v AlbumsView) clampCursor() AlbumsView {
	var n int
	switch v.level {
	case albtui.TrackLevel:
		n = len(v.tracks)
	default:
		n = len(v.albums)
	}
	if n == 0 {
		v.cursor = 0
		v.scrollOffset = 0
	} else if v.cursor >= n {
		v.cursor = n - 1
	}
	return v
}

// ActivateFilter sets the filter bar to active input mode.
func (v AlbumsView) ActivateFilter() AlbumsView {
	v.filter = v.filter.Activate()
	return v
}

// CommitFilter exits input mode while keeping the current query visible.
func (v AlbumsView) CommitFilter() AlbumsView {
	v.filter = v.filter.Commit()
	return v
}

// DeactivateFilter clears the query, exits input mode, and resets the cursor.
func (v AlbumsView) DeactivateFilter() AlbumsView {
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	return v
}

// AppendFilterChar adds ch to the filter text and resets the cursor.
func (v AlbumsView) AppendFilterChar(ch rune) AlbumsView {
	var changed bool
	v.filter, changed = v.filter.AppendChar(ch)
	if changed {
		v.cursor = 0
		v.scrollOffset = 0
	}
	return v
}

// BackspaceFilter removes the last rune from the filter text.
func (v AlbumsView) BackspaceFilter() AlbumsView {
	var changed bool
	v.filter, changed = v.filter.Backspace()
	if changed {
		v.cursor = 0
		v.scrollOffset = 0
	}
	return v
}

// focusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) focusedAlbum() (string, bool) {
	if v.level != albtui.AlbumLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.albums, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedTrack returns the track at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) focusedTrack() (musiclib.Track, bool) {
	if v.level != albtui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.tracks, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.cursor], true
}

// enqueueFocusedTrack returns a tea.Cmd that appends the focused track to the
// play queue via qa. Returns nil when no track is focused.
func (v AlbumsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	if !ok {
		return nil
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		id, err := qa.AppendTrack(track, "")
		return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
}
