package playlists

import (
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// PlaylistsView wraps the playlists sub-panel state for the root model.
// It holds the navigation level, cursor position, loaded playlist rows, and
// (at TrackLevel) the tracks of the open playlist. Rendering is delegated to
// View.
type PlaylistsView struct {
	Level            Level
	Cursor           int
	ListCursor       int // saved list cursor; preserved across drill-down and back
	listScrollOffset int // saved list scroll offset; preserved across drill-down and back
	ScrollOffset     int // first visible row index
	Playlists        []db.PlaylistRow
	Tracks           []db.PlaylistTrackRow // populated when Level == TrackLevel
	Width            int
	Height           int
	Creating         bool               // true while the inline create input is active
	createInput      textinput.Model    // current value of the inline create field
	Renaming         bool               // true while the inline rename input is active
	RenameInput      textinput.Model    // current value of the inline rename field
	SelectedTrackIDs map[int64]struct{} // bulk-selected playlist track row IDs at TrackLevel
	SortState        sorting.SortState
	SortedPlaylists  []db.PlaylistRow       // sorted copy for list level (nil when default)
	SortedTrackOrder []int                  // index permutation for track level (nil when default)
	SmartQuery       *db.SmartPlaylistQuery // non-nil when TrackLevel shows a smart playlist
	PendingLoadID    int64                  // playlist ID of a pending async track load; 0 when idle
}

func (v *PlaylistsView) SetSize(w, h int) { v.Width = w; v.Height = h }

// EffectivePlaylists returns the sorted playlists when sort is active,
// otherwise the original playlists.
func (v PlaylistsView) EffectivePlaylists() []db.PlaylistRow {
	if v.SortedPlaylists != nil {
		return v.SortedPlaylists
	}
	return v.Playlists
}

// DisplayTracks returns the playlist's tracks in display order. Unlike
// TagsView.displayTracks there is no filter step: PlaylistsView carries no
// filter state and root_filter.go has no viewPlaylists case.
//
// A stale SortedTrackOrder (length mismatch or out-of-range index) falls back
// to natural order — reordering through a partially-filled or out-of-range
// permutation would show empty rows or panic.
func (v PlaylistsView) DisplayTracks() []db.PlaylistTrackRow {
	if v.SortedTrackOrder == nil || len(v.SortedTrackOrder) != len(v.Tracks) {
		return v.Tracks
	}
	reordered := make([]db.PlaylistTrackRow, len(v.Tracks))
	for i, idx := range v.SortedTrackOrder {
		if idx < 0 || idx >= len(v.Tracks) {
			return v.Tracks
		}
		reordered[i] = v.Tracks[idx]
	}
	return reordered
}

// effectiveTrack returns the track at the given display index, translating
// through SortedTrackOrder when active.
func (v PlaylistsView) TrackIndex(displayIdx int) int {
	if v.SortedTrackOrder == nil {
		return displayIdx
	}
	if displayIdx < 0 || displayIdx >= len(v.SortedTrackOrder) {
		return displayIdx
	}
	return v.SortedTrackOrder[displayIdx]
}

// FindTrackDisplayPosition returns the display row for the given underlying
// track index. Returns 0 if not found.
func (v PlaylistsView) FindTrackDisplayPosition(trackIdx int) int {
	if v.SortedTrackOrder == nil {
		return trackIdx
	}
	for i, idx := range v.SortedTrackOrder {
		if idx == trackIdx {
			return i
		}
	}
	return 0
}

// RecomputePlaylistSort rebuilds the sorted playlist/track caches.
func (v PlaylistsView) RecomputePlaylistSort() PlaylistsView {
	if v.Level == ListLevel {
		v.SortedPlaylists = sorting.SortPlaylists(v.Playlists, v.SortState.Field, v.SortState.Dir)
		v.SortedTrackOrder = nil
	} else {
		v.SortedPlaylists = nil
		v.SortedTrackOrder = sorting.SortPlaylistTrackIndices(v.Tracks, v.SortState.Field, v.SortState.Dir)
	}
	return v
}
