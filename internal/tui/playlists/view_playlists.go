package playlists

import (
	"strings"

	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// PlaylistsView wraps the playlists sub-panel state for the root model.
// It holds the navigation level, cursor position, loaded playlist rows, and
// (at TrackLevel) the tracks of the open playlist. Rendering is delegated to
// View.
type PlaylistsView struct {
	Filter           filter.State
	listFilter       filter.State
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

// EffectivePlaylists returns playlists in their visible sort and filter order.
func (v PlaylistsView) EffectivePlaylists() []db.PlaylistRow {
	rows := v.Playlists
	if v.SortedPlaylists != nil {
		rows = v.SortedPlaylists
	}
	if v.Filter.Text == "" {
		return rows
	}
	query := strings.ToLower(v.Filter.Text)
	var out []db.PlaylistRow
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), query) {
			out = append(out, row)
		}
	}
	return out
}

// displayTrackIndices retains canonical row indices, including duplicate songs.
// Stale sort permutations fall back to natural order before filtering.
func (v PlaylistsView) displayTrackIndices() []int {
	order := v.SortedTrackOrder
	if !v.validTrackOrder() {
		order = make([]int, len(v.Tracks))
		for i := range order {
			order[i] = i
		}
	}
	if v.Filter.Text == "" {
		return order
	}
	query := library.ParseFilterQuery(v.Filter.Text)
	filtered := make([]int, 0, len(order))
	for _, idx := range order {
		row := v.Tracks[idx]
		if query.MatchesMetadata(row.Title, row.Artist, row.Album, row.Genre) {
			filtered = append(filtered, idx)
		}
	}
	return filtered
}

// DisplayTracks returns the visible rows without losing playlist row IDs.
func (v PlaylistsView) DisplayTracks() []db.PlaylistTrackRow {
	if v.Filter.Text == "" && v.SortedTrackOrder == nil {
		return v.Tracks
	}
	order := v.displayTrackIndices()
	rows := make([]db.PlaylistTrackRow, len(order))
	for i, idx := range order {
		rows[i] = v.Tracks[idx]
	}
	return rows
}

// TrackIndex translates a visible row to its canonical index, or -1.
func (v PlaylistsView) TrackIndex(displayIdx int) int {
	if displayIdx < 0 || displayIdx >= len(v.Tracks) {
		return -1
	}
	if v.Filter.Text == "" && v.SortedTrackOrder == nil {
		return displayIdx
	}
	order := v.displayTrackIndices()
	if displayIdx < 0 || displayIdx >= len(order) {
		return -1
	}
	return order[displayIdx]
}

func (v PlaylistsView) FindTrackDisplayPosition(trackIdx int) int {
	for i, idx := range v.displayTrackIndices() {
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

func (v PlaylistsView) validTrackOrder() bool {
	if v.SortedTrackOrder == nil || len(v.SortedTrackOrder) != len(v.Tracks) {
		return false
	}
	for _, idx := range v.SortedTrackOrder {
		if idx < 0 || idx >= len(v.Tracks) {
			return false
		}
	}
	return true
}
