package library

import (
	"time"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// SwitchMode transitions the view to the given mode, saving the current
// cursor/scroll/filter and restoring the saved state for the target mode.
// sourceTracks for History mode must be populated by the caller after this
// returns (it requires the historyReader which View does not hold).
func (v View) SwitchMode(to LibraryMode) View {
	v.modeState[v.Mode] = libraryModeSnapshot{
		cursor: v.Cursor,
		scroll: v.ScrollOffset,
		filter: v.Filter,
	}
	v.Mode = to
	saved := v.modeState[to]
	v.Cursor = saved.cursor
	v.ScrollOffset = saved.scroll
	v.Filter = saved.filter
	v.FilteredMatches = nil
	v.SelectedHashes = nil

	switch to {
	case LibraryModeAll:
		v.ShowDateAdded = false
		v.ShowPlayedAt = false
		v.SourceTracks = nil
		v.SortState = sorting.SortState{}
		v.SortedTracks = nil
	case LibraryModeRecent:
		v.ShowDateAdded = true
		v.ShowPlayedAt = false
		v.SortState = sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}
		v.SortedTracks = nil
	case LibraryModeHistory:
		v.ShowDateAdded = false
		v.ShowPlayedAt = true
		v.SortState = sorting.SortState{Field: sorting.SortLastPlayed, Dir: sorting.SortDesc}
		v.SortedTracks = nil
	}

	if v.Filter.Text != "" {
		v = v.RebuildFilterCache()
	}
	return v
}

func (v View) BaseTracks() []musiclib.Track {
	var all []musiclib.Track
	if v.SourceTracks != nil {
		all = v.SourceTracks
	} else {
		all = v.Index.Tracks()
	}
	if v.TagFilterHashes == nil {
		return all
	}
	filtered := make([]musiclib.Track, 0, len(all))
	for _, t := range all {
		if _, ok := v.TagFilterHashes[t.Hash]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// Panel returns a fully-populated Panel from the current View state.
func (v View) Panel() Panel {
	lv := Panel{
		Index:           v.Index,
		Width:           v.Width,
		Height:          v.Height,
		Cursor:          v.Cursor,
		ScrollOffset:    v.ScrollOffset,
		Filter:          v.Filter,
		FilteredMatches: v.FilteredMatches,
		TagFilterActive: v.TagFilterHashes != nil,
		Fuzzy:           v.Fuzzy,
		SelectedHashes:  v.SelectedHashes,
		ShowDateAdded:   v.ShowDateAdded,
		ShowPlayedAt:    v.ShowPlayedAt,
		StripViewLabel:  StripLabel(v.Mode),
	}
	if v.Label != "" {
		lv.StripViewLabel = v.Label
	}
	switch {
	case v.SortState.Field != sorting.SortDefault:
		lv.OverrideTracks = v.SortedTracks
		if lv.OverrideTracks == nil {
			// A nil sort cache must not bypass source or tag restrictions.
			lv.OverrideTracks = v.BaseTracks()
		}
		lv.SortField = int(v.SortState.Field)
		lv.SortDir = int(v.SortState.Dir)
		lv.StripSortLabel = sorting.SortFieldLabel(v.SortState.Field)
		lv.StripSortIndicator = sorting.SortDirIndicator(v.SortState.Dir)
	case v.SourceTracks != nil || v.TagFilterHashes != nil:
		lv.OverrideTracks = v.BaseTracks()
	}
	if sorting.SortFieldHasBuckets(v.SortState.Field) {
		field := v.SortState.Field
		now := time.Now()
		lv.BucketLabel = func(t musiclib.Track) string {
			return sorting.TrackBucket(field, t, now)
		}
	}
	return lv
}

// CycleSortField advances to the next sort field from the given list, resets
// the cursor, and recomputes the sorted/filtered caches. It returns the
// updated view and the new SortState for notification purposes.
func (v View) CycleSortField(fields []sorting.SortField) (View, sorting.SortState) {
	next := sorting.CycleSortField(v.SortState.Field, fields)
	v.SortState.Field = next
	if next == sorting.SortDefault {
		v.SortState.Dir = sorting.SortAsc
	}
	v = v.RefreshView()
	v.Cursor = 0
	v.ScrollOffset = 0
	return v, v.SortState
}

// ApplySortDir sets the sort direction, preserving the focused track position.
// Returns (updated view, sort state, true) when applied, or (v, zero, false)
// when the current sort field is SortDefault.
func (v View) ApplySortDir(dir sorting.SortDir) (View, sorting.SortState, bool) {
	if v.SortState.Field == sorting.SortDefault {
		return v, sorting.SortState{}, false
	}
	var focusedHash string
	if t, ok := v.Panel().FocusedTrack(); ok {
		focusedHash = t.Hash
	}
	v.SortState.Dir = dir
	v = v.RefreshView()
	v.Cursor = v.FindTrackPosition(focusedHash)
	v.ScrollOffset = sorting.AdjustScroll(v.Cursor, v.ScrollOffset, layout.VisibleRows(v.Height, VisibleReserve(v)))
	return v, v.SortState, true
}

// RecomputeSortedTracks rebuilds the sorted tracks cache based on the current
// sort state. When sorting is default, the cache is cleared.
func (v View) RecomputeSortedTracks() View {
	if v.SortState.Field == sorting.SortDefault {
		v.SortedTracks = nil
		return v
	}
	v.SortedTracks = sorting.SortTracks(v.BaseTracks(), v.SortState.Field, v.SortState.Dir)
	return v
}

// RebuildFilterCache recomputes filteredMatches for the current filterText.
// Call this whenever filterText changes so navigation and rendering reuse the
// cached slice instead of re-running the fuzzy search on every frame.
func (v View) RebuildFilterCache() View {
	if v.Filter.Text != "" && v.Fuzzy != nil {
		tracks := v.BaseTracks()
		if v.SortedTracks != nil {
			tracks = v.SortedTracks
		}
		v.FilteredMatches = v.Fuzzy.Search(v.Filter.Text, tracks)
	} else {
		v.FilteredMatches = nil
	}
	return v
}

// RefreshView recomputes sort and filter caches in one call. Both underlying
// methods are no-ops when sort is default or filter is empty respectively.
func (v View) RefreshView() View {
	return v.RecomputeSortedTracks().RebuildFilterCache()
}

// FindTrackPosition returns the cursor position for a track with the given hash
// in the current effective track list (sorted + filtered). Returns 0 if not found.
func (v View) FindTrackPosition(hash string) int {
	if hash == "" {
		return 0
	}
	tracks := v.SortedTracks
	if tracks == nil {
		tracks = v.BaseTracks()
	}
	if v.Filter.Text != "" && v.FilteredMatches != nil {
		return matchPositionOf(v.FilteredMatches, tracks, hash)
	}
	return trackPositionOf(tracks, hash)
}

// matchPositionOf returns the index of the match whose track carries hash, or 0
// when no match does.
func matchPositionOf(matches []musiclib.Match, tracks []musiclib.Track, hash string) int {
	for i, m := range matches {
		if m.TrackIndex >= 0 && m.TrackIndex < len(tracks) && tracks[m.TrackIndex].Hash == hash {
			return i
		}
	}
	return 0
}

// trackPositionOf returns the index of the track carrying hash, or 0 when none
// does.
func trackPositionOf(tracks []musiclib.Track, hash string) int {
	for i, t := range tracks {
		if t.Hash == hash {
			return i
		}
	}
	return 0
}

func (v View) TrackCount() int {
	if v.Filter.Text != "" && v.FilteredMatches != nil {
		return len(v.FilteredMatches)
	}
	if v.SortedTracks != nil {
		return len(v.SortedTracks)
	}
	return len(v.BaseTracks())
}

func (v View) ClampCursor() View {
	count := v.TrackCount()
	if count == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor >= count {
		v.Cursor = count - 1
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	v.ScrollOffset = sorting.AdjustScroll(v.Cursor, v.ScrollOffset, layout.VisibleRows(v.Height, VisibleReserve(v)))
	return v
}
