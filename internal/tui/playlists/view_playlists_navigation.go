package playlists

import (
	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
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

// HandleAction executes a pre-resolved playlists action dispatched by root.Update.
func (v PlaylistsView) HandleAction(a keys.Action) PlaylistsView {
	if a == keys.ActionBulkSelectionToggle {
		if v.Level == TrackLevel && len(v.Tracks) > 0 && v.Cursor < len(v.Tracks) {
			id := v.Tracks[v.TrackIndex(v.Cursor)].ID
			view := View{SelectedTrackIDs: v.SelectedTrackIDs}.ToggleBulkSelection(id)
			v.SelectedTrackIDs = view.SelectedTrackIDs
		}
		return v
	}
	var sortField, sortDir string
	if v.SortState.Field != sorting.SortDefault {
		sortField = sorting.SortFieldLabel(v.SortState.Field)
		sortDir = sorting.SortDirIndicator(v.SortState.Dir)
	}
	view := View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
		Height:       v.Height,
		Playlists:    v.Playlists,
		Tracks:       v.Tracks,
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
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
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
	v.Cursor, v.ScrollOffset = layout.ClampAfterRemoval(v.Cursor, v.ScrollOffset, len(tracks))
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
	if v.SortedPlaylists != nil && v.Cursor >= 0 && v.Cursor < len(effective) {
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
	return v
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
	if v.Cursor < 0 || v.Cursor >= len(v.Tracks) {
		return musiclib.Track{}, false
	}
	t := v.Tracks[v.TrackIndex(v.Cursor)]
	return musiclib.Track{
		Hash:     t.Hash,
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Album:    t.Album,
		Duration: float64(t.DurationMs) / 1000,
	}, true
}
