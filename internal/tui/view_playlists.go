package tui

import (
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

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
	sortState        sorting.SortState
	sortedPlaylists  []db.PlaylistRow       // sorted copy for list level (nil when default)
	sortedTrackOrder []int                  // index permutation for track level (nil when default)
	smartQuery       *db.SmartPlaylistQuery // non-nil when TrackLevel shows a smart playlist
	pendingLoadID    int64                  // playlist ID of a pending async track load; 0 when idle
}

func (v *PlaylistsView) SetSize(w, h int) { v.width = w; v.height = h }

// effectivePlaylists returns the sorted playlists when sort is active,
// otherwise the original playlists.
func (v PlaylistsView) effectivePlaylists() []db.PlaylistRow {
	if v.sortedPlaylists != nil {
		return v.sortedPlaylists
	}
	return v.playlists
}

// displayTracks returns the playlist's tracks in display order. Unlike
// TagsView.displayTracks there is no filter step: PlaylistsView carries no
// filter state and root_filter.go has no viewPlaylists case.
//
// A stale sortedTrackOrder (length mismatch or out-of-range index) falls back
// to natural order — reordering through a partially-filled or out-of-range
// permutation would show empty rows or panic.
func (v PlaylistsView) displayTracks() []db.PlaylistTrackRow {
	if v.sortedTrackOrder == nil || len(v.sortedTrackOrder) != len(v.tracks) {
		return v.tracks
	}
	reordered := make([]db.PlaylistTrackRow, len(v.tracks))
	for i, idx := range v.sortedTrackOrder {
		if idx < 0 || idx >= len(v.tracks) {
			return v.tracks
		}
		reordered[i] = v.tracks[idx]
	}
	return reordered
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
		v.sortedPlaylists = sorting.SortPlaylists(v.playlists, v.sortState.Field, v.sortState.Dir)
		v.sortedTrackOrder = nil
	} else {
		v.sortedPlaylists = nil
		v.sortedTrackOrder = sorting.SortPlaylistTrackIndices(v.tracks, v.sortState.Field, v.sortState.Dir)
	}
	return v
}
