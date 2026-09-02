package tui

import (
	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

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
func (v PlaylistsView) HandleAction(a keys.Action) PlaylistsView {
	if a == keys.ActionBulkSelectionToggle {
		if v.level == pltui.TrackLevel && len(v.tracks) > 0 && v.cursor < len(v.tracks) {
			id := v.tracks[v.trackIndex(v.cursor)].ID
			view := pltui.View{SelectedTrackIDs: v.selectedTrackIDs}.ToggleBulkSelection(id)
			v.selectedTrackIDs = view.SelectedTrackIDs
		}
		return v
	}
	var sortField, sortDir string
	if v.sortState.Field != sorting.SortDefault {
		sortField = sorting.SortFieldLabel(v.sortState.Field)
		sortDir = sorting.SortDirIndicator(v.sortState.Dir)
	}
	view := pltui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
		Height:       v.height,
		Playlists:    v.playlists,
		Tracks:       v.tracks,
		SortField:    sortField,
		SortDir:      sortDir,
	}
	switch a {
	case keys.ActionCursorUp:
		view = view.CursorUp()
	case keys.ActionCursorDown:
		view = view.CursorDown()
	case keys.ActionHalfPageDown:
		view = view.HalfPageDown()
	case keys.ActionHalfPageUp:
		view = view.HalfPageUp()
	case keys.ActionFullPageDown:
		view = view.FullPageDown()
	case keys.ActionFullPageUp:
		view = view.FullPageUp()
	case keys.ActionGoToTop:
		view = view.GoToTop()
	case keys.ActionGoToBottom:
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
	v.sortState = sorting.SortState{}
	v.sortedPlaylists = nil
	v.sortedTrackOrder = nil
	return v
}

// backToList transitions the view back to ListLevel, restoring the list cursor
// to the position it held before the drill-down.
func (v PlaylistsView) backToList() PlaylistsView {
	view := pltui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ListCursor:       v.listCursor,
		ListScrollOffset: v.listScrollOffset,
		Playlists:        v.effectivePlaylists(),
		Height:           v.height,
	}.BackToList()
	v.level = view.Level
	v.cursor = view.Cursor
	v.tracks = view.Tracks
	v.scrollOffset = view.ScrollOffset
	v.selectedTrackIDs = nil // clear stale track-level selections
	v.sortState = sorting.SortState{}
	v.sortedPlaylists = nil
	v.sortedTrackOrder = nil
	v.smartQuery = nil
	v.pendingLoadID = 0 // discard any in-flight async track load
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
