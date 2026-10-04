package playlists

import (
	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// FocusedPlaylist returns the playlist at the current cursor position and true.
// Returns a zero value and false when the list is empty or not at ListLevel.
func (v PlaylistsView) FocusedPlaylist() (db.PlaylistRow, bool) {
	pl := v.EffectivePlaylists()
	if len(pl) == 0 || v.Level != ListLevel {
		return db.PlaylistRow{}, false
	}
	if v.Cursor < 0 || v.Cursor >= len(pl) {
		return db.PlaylistRow{}, false
	}
	return pl[v.Cursor], true
}

// navigationMoves maps a cursor-movement action to the View method performing
// it. Actions absent from the table leave the cursor untouched.
var navigationMoves = map[keys.Action]func(View) View{
	keys.ActionCursorUp:     View.CursorUp,
	keys.ActionCursorDown:   View.CursorDown,
	keys.ActionHalfPageDown: View.HalfPageDown,
	keys.ActionHalfPageUp:   View.HalfPageUp,
	keys.ActionFullPageDown: View.FullPageDown,
	keys.ActionFullPageUp:   View.FullPageUp,
	keys.ActionGoToTop:      View.GoToTop,
	keys.ActionGoToBottom:   View.GoToBottom,
}

// HandleAction executes a pre-resolved playlists action dispatched by root.Update.
func (v PlaylistsView) HandleAction(a keys.Action) PlaylistsView {
	if a == keys.ActionBulkSelectionToggle {
		return v.toggleFocusedTrackSelection()
	}
	move, ok := navigationMoves[a]
	if !ok {
		return v
	}
	view := move(v.navigationView())
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
}

// toggleFocusedTrackSelection flips the bulk-selection marker on the focused
// track. It is a no-op anywhere but on a non-empty track list.
func (v PlaylistsView) toggleFocusedTrackSelection() PlaylistsView {
	idx := v.TrackIndex(v.Cursor)
	if v.Level != TrackLevel || idx < 0 {
		return v
	}
	id := v.Tracks[idx].ID
	view := View{SelectedTrackIDs: v.SelectedTrackIDs}.ToggleBulkSelection(id)
	v.SelectedTrackIDs = view.SelectedTrackIDs
	return v
}

// navigationView projects the playlists view onto the plain View that owns the
// cursor-movement methods, resolving the sort labels those methods read.
func (v PlaylistsView) navigationView() View {
	var sortField, sortDir string
	if v.SortState.Field != sorting.SortDefault {
		sortField = sorting.SortFieldLabel(v.SortState.Field)
		sortDir = sorting.SortDirIndicator(v.SortState.Dir)
	}
	return View{
		Level:         v.Level,
		Cursor:        v.Cursor,
		ScrollOffset:  v.ScrollOffset,
		Height:        v.Height,
		Playlists:     v.EffectivePlaylists(),
		Tracks:        v.DisplayTracks(),
		FilterVisible: v.Filter.Active || v.Filter.Text != "",
		SortField:     sortField,
		SortDir:       sortDir,
	}
}

// HasTrackSelection reports whether any tracks are bulk-selected.
func (v PlaylistsView) HasTrackSelection() bool {
	return len(v.SelectedTrackIDs) > 0
}

// ClearTrackSelection removes all bulk-selection markers.
func (v PlaylistsView) ClearTrackSelection() PlaylistsView {
	v.SelectedTrackIDs = nil
	return v
}

// EnsureCursorVisible adjusts ScrollOffset so the cursor row is within the
// visible window. Call after operations that reposition the cursor (e.g.
// playlist rename or create) where the new cursor position may fall outside
// the current viewport.
func (v PlaylistsView) EnsureCursorVisible() PlaylistsView {
	if v.Filter.Text != "" {
		count := len(v.EffectivePlaylists())
		if v.Level == TrackLevel {
			count = len(v.DisplayTracks())
		}
		v.Cursor, v.ScrollOffset = layout.ClampAfterRemoval(v.Cursor, v.ScrollOffset, count)

	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else {
		reserve := 3
		rows := FallbackVisibleRows
		if v.Height > 0 {
			rows = max(1, v.Height-reserve)
		}
		if v.Cursor >= v.ScrollOffset+rows {
			v.ScrollOffset = v.Cursor - rows + 1
		}
	}
	return v
}

// RefreshTracks updates the track slice while staying at TrackLevel and clamps
// the cursor to the new length. Unlike OpenTracks it does not reset the cursor
// to 0 or save the list cursor, so it is suitable for in-place re-evaluation
// of a smart playlist query.
func (v PlaylistsView) RefreshTracks(tracks []db.PlaylistTrackRow) PlaylistsView {
	v.Tracks = tracks
	v.SortedTrackOrder = nil
	v.Cursor, v.ScrollOffset = layout.ClampAfterRemoval(v.Cursor, v.ScrollOffset, len(v.DisplayTracks()))
	return v
}

// OpenTracks transitions the view to TrackLevel, storing the loaded tracks
// and resetting the cursor to the first row. The current list cursor is saved
// so it can be restored by BackToList.
func (v PlaylistsView) OpenTracks(tracks []db.PlaylistTrackRow) PlaylistsView {
	// Translate the display cursor to a canonical (unsorted) index so that
	// ListCursor always indexes into v.Playlists correctly, even when a
	// sort is active at list level.
	canonicalCursor := v.Cursor
	effective := v.EffectivePlaylists()
	if (v.SortedPlaylists != nil || v.Filter.Text != "") && v.Cursor >= 0 && v.Cursor < len(effective) {
		focusedID := effective[v.Cursor].ID
		for i, p := range v.Playlists {
			if p.ID == focusedID {
				canonicalCursor = i
				break
			}
		}
	}
	view := View{
		Level:      v.Level,
		Cursor:     canonicalCursor,
		ListCursor: v.ListCursor,
	}.OpenTracks(tracks)
	v.listFilter = v.Filter
	v.Filter = filter.State{}
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ListCursor = view.ListCursor
	v.listScrollOffset = v.ScrollOffset // preserve list-level scroll position
	v.Tracks = view.Tracks
	v.ScrollOffset = 0
	v.SelectedTrackIDs = nil // clear stale track-level selections from a previous playlist
	v.SortState = sorting.SortState{}
	v.SortedPlaylists = nil
	v.SortedTrackOrder = nil
	return v
}

// BackToList transitions the view back to ListLevel, restoring the list cursor
// to the position it held before the drill-down.
func (v PlaylistsView) BackToList() PlaylistsView {
	var focusedID int64
	if v.ListCursor >= 0 && v.ListCursor < len(v.Playlists) {
		focusedID = v.Playlists[v.ListCursor].ID
	}
	v.Filter = v.listFilter
	v.listFilter = filter.State{}
	v.SortedPlaylists = nil
	view := View{
		Level:            v.Level,
		Cursor:           v.Cursor,
		ListCursor:       v.ListCursor,
		ListScrollOffset: v.listScrollOffset,
		Playlists:        v.EffectivePlaylists(),
		Height:           v.Height,
	}.BackToList()
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.Tracks = view.Tracks
	v.ScrollOffset = view.ScrollOffset
	v.SelectedTrackIDs = nil // clear stale track-level selections
	v.SortState = sorting.SortState{}
	v.SortedPlaylists = nil
	v.SortedTrackOrder = nil
	v.SmartQuery = nil
	v.PendingLoadID = 0 // discard any in-flight async track load
	if v.Filter.Text != "" && focusedID != 0 {
		v.Cursor = sorting.FindPlaylistPosition(v.EffectivePlaylists(), focusedID)
	}
	return v.EnsureCursorVisible()
}

// AtListLevel reports whether the view is currently showing the playlist list.
func (v PlaylistsView) AtListLevel() bool {
	return v.Level == ListLevel
}

// FocusedTrack returns the musiclib.Track for the cursor position at TrackLevel.
// Returns false when the view is not at TrackLevel or the track list is empty.
func (v PlaylistsView) FocusedTrack() (musiclib.Track, bool) {
	if v.Level != TrackLevel || len(v.Tracks) == 0 {
		return musiclib.Track{}, false
	}
	idx := v.TrackIndex(v.Cursor)
	if idx < 0 {
		return musiclib.Track{}, false
	}
	t := v.Tracks[idx]
	return musiclib.Track{
		Hash:     t.Hash,
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Album:    t.Album,
		Duration: float64(t.DurationMs) / 1000,
	}, true
}
