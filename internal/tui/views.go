package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
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
	viewFavorites viewID = 6
	viewGenres    viewID = 7
)

// viewOrder defines the tab-cycling order for primary views.
var viewOrder = []viewID{viewLibrary, viewPlaylists, viewQueue, viewArtists, viewAlbums, viewGenres, viewFavorites}

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
	case viewFavorites:
		return "Favorites"
	case viewGenres:
		return "Genres"
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
	case viewFavorites:
		return contextFavorites
	case viewGenres:
		return contextGenres
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
	sortState        SortState
	sortedPlaylists  []db.PlaylistRow       // sorted copy for list level (nil when default)
	sortedTrackOrder []int                  // index permutation for track level (nil when default)
	smartQuery       *db.SmartPlaylistQuery // non-nil when TrackLevel shows a smart playlist
}

func (v *PlaylistsView) setSize(w, h int) { v.width = w; v.height = h }

// effectivePlaylists returns the sorted playlists when sort is active,
// otherwise the original playlists.
func (v PlaylistsView) effectivePlaylists() []db.PlaylistRow {
	if v.sortedPlaylists != nil {
		return v.sortedPlaylists
	}
	return v.playlists
}

// effectiveTrack returns the track at the given display index, translating
// through sortedTrackOrder when active.
func (v PlaylistsView) trackIndex(displayIdx int) int {
	if v.sortedTrackOrder == nil {
		return displayIdx
	}
	if displayIdx < 0 || displayIdx >= len(v.sortedTrackOrder) {
		return displayIdx
	}
	return v.sortedTrackOrder[displayIdx]
}

// findTrackDisplayPosition returns the display row for the given underlying
// track index. Returns 0 if not found.
func (v PlaylistsView) findTrackDisplayPosition(trackIdx int) int {
	if v.sortedTrackOrder == nil {
		return trackIdx
	}
	for i, idx := range v.sortedTrackOrder {
		if idx == trackIdx {
			return i
		}
	}
	return 0
}

// recomputePlaylistSort rebuilds the sorted playlist/track caches.
func (v PlaylistsView) recomputePlaylistSort() PlaylistsView {
	if v.level == pltui.ListLevel {
		v.sortedPlaylists = SortPlaylists(v.playlists, v.sortState.Field, v.sortState.Dir)
		v.sortedTrackOrder = nil
	} else {
		v.sortedPlaylists = nil
		v.sortedTrackOrder = SortPlaylistTrackIndices(v.tracks, v.sortState.Field, v.sortState.Dir)
	}
	return v
}

func (v PlaylistsView) render(s styles.Styles, favHashes map[string]struct{}) string {
	playlists := v.playlists
	if v.sortedPlaylists != nil {
		playlists = v.sortedPlaylists
	}
	tracks := v.tracks
	if v.sortedTrackOrder != nil {
		// Build a reordered track slice for rendering.
		reordered := make([]db.PlaylistTrackRow, len(v.tracks))
		for i, idx := range v.sortedTrackOrder {
			reordered[i] = v.tracks[idx]
		}
		tracks = reordered
	}
	pv := pltui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Playlists:        playlists,
		Tracks:           tracks,
		Width:            v.width,
		Height:           v.height,
		Creating:         v.creating,
		CreateInput:      v.createInput,
		Renaming:         v.renaming,
		RenameInput:      v.renameInput,
		SelectedTrackIDs: v.selectedTrackIDs,
		IsSmartPlaylist:  v.smartQuery != nil,
	}
	pv.Styles = s
	pv.FavoriteHashes = favHashes
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

// createInputValue returns the current create input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) createInputValue() string {
	return strings.TrimSpace(v.createInput.Value())
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

// renameInputValue returns the current rename input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) renameInputValue() string {
	return strings.TrimSpace(v.renameInput.Value())
}

// renameInputIsEmpty reports whether the trimmed rename input buffer is empty.
func (v PlaylistsView) renameInputIsEmpty() bool {
	return strings.TrimSpace(v.renameInput.Value()) == ""
}

// focusedPlaylist returns the playlist at the current cursor position and true.
// Returns a zero value and false when the list is empty or not at ListLevel.
func (v PlaylistsView) focusedPlaylist() (db.PlaylistRow, bool) {
	pl := v.effectivePlaylists()
	if len(pl) == 0 || v.level != pltui.ListLevel {
		return db.PlaylistRow{}, false
	}
	if v.cursor < 0 || v.cursor >= len(pl) {
		return db.PlaylistRow{}, false
	}
	return pl[v.cursor], true
}

// HandleAction executes a pre-resolved playlists action dispatched by root.Update.
func (v PlaylistsView) HandleAction(a action) PlaylistsView {
	if a == actionBulkSelectionToggle {
		if v.level == pltui.TrackLevel && len(v.tracks) > 0 && v.cursor < len(v.tracks) {
			id := v.tracks[v.trackIndex(v.cursor)].ID
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
		rows := playlistFallbackVisibleRows
		if v.height > 0 {
			rows = max(1, v.height-reserve)
		}
		if v.cursor >= v.scrollOffset+rows {
			v.scrollOffset = v.cursor - rows + 1
		}
	}
	return v
}

// focusedPlaylistID returns the ID of the playlist at the current cursor
// position and true. Returns 0, false when the list is empty or the view is
// not at ListLevel.
func (v PlaylistsView) focusedPlaylistID() (int64, bool) {
	pl := v.effectivePlaylists()
	if len(pl) == 0 || v.level != pltui.ListLevel {
		return 0, false
	}
	if v.cursor < 0 || v.cursor >= len(pl) {
		return 0, false
	}
	return pl[v.cursor].ID, true
}

// refreshTracks updates the track slice while staying at TrackLevel and clamps
// the cursor to the new length. Unlike openTracks it does not reset the cursor
// to 0 or save the list cursor, so it is suitable for in-place re-evaluation
// of a smart playlist query.
func (v PlaylistsView) refreshTracks(tracks []db.PlaylistTrackRow) PlaylistsView {
	v.tracks = tracks
	v.sortedTrackOrder = nil
	v.cursor, v.scrollOffset = clampAfterRemoval(v.cursor, v.scrollOffset, len(tracks))
	return v
}

// openTracks transitions the view to TrackLevel, storing the loaded tracks
// and resetting the cursor to the first row. The current list cursor is saved
// so it can be restored by backToList.
func (v PlaylistsView) openTracks(tracks []db.PlaylistTrackRow) PlaylistsView {
	// Translate the display cursor to a canonical (unsorted) index so that
	// listCursor always indexes into v.playlists correctly, even when a
	// sort is active at list level.
	canonicalCursor := v.cursor
	effective := v.effectivePlaylists()
	if v.sortedPlaylists != nil && v.cursor >= 0 && v.cursor < len(effective) {
		focusedID := effective[v.cursor].ID
		for i, p := range v.playlists {
			if p.ID == focusedID {
				canonicalCursor = i
				break
			}
		}
	}
	view := pltui.View{
		Level:      v.level,
		Cursor:     canonicalCursor,
		ListCursor: v.listCursor,
	}.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.listCursor = view.ListCursor
	v.listScrollOffset = v.scrollOffset // preserve list-level scroll position
	v.tracks = view.Tracks
	v.scrollOffset = 0
	v.selectedTrackIDs = nil // clear stale track-level selections from a previous playlist
	v.sortState = SortState{}
	v.sortedPlaylists = nil
	v.sortedTrackOrder = nil
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
	v.selectedTrackIDs = nil            // clear stale track-level selections
	v.sortState = SortState{}
	v.sortedPlaylists = nil
	v.sortedTrackOrder = nil
	v.smartQuery = nil
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
	t := v.tracks[v.trackIndex(v.cursor)]
	return musiclib.Track{
		Hash:     t.Hash,
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Album:    t.Album,
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
