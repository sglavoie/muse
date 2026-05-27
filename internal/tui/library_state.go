package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// switchMode transitions the view to the given mode, saving the current
// cursor/scroll/filter and restoring the saved state for the target mode.
// sourceTracks for History mode must be populated by the caller after this
// returns (it requires the historyReader which LibraryView does not hold).
func (v LibraryView) switchMode(to LibraryMode) LibraryView {
	v.modeState[v.mode] = libraryModeSnapshot{
		cursor: v.cursor,
		scroll: v.scrollOffset,
		filter: v.filter,
	}
	v.mode = to
	saved := v.modeState[to]
	v.cursor = saved.cursor
	v.scrollOffset = saved.scroll
	v.filter = saved.filter
	v.filteredMatches = nil
	v.selectedHashes = nil

	switch to {
	case LibraryModeAll:
		v.showDateAdded = false
		v.showPlayedAt = false
		v.sourceTracks = nil
		v.sortState = SortState{}
		v.sortedTracks = nil
	case LibraryModeRecent:
		v.showDateAdded = true
		v.showPlayedAt = false
		v.sortState = SortState{Field: SortDateAdded, Dir: SortDesc}
		v.sortedTracks = nil
	case LibraryModeHistory:
		v.showDateAdded = false
		v.showPlayedAt = true
		v.sortState = SortState{Field: SortLastPlayed, Dir: SortDesc}
		v.sortedTracks = nil
	}

	if v.filter.Text != "" {
		v = v.rebuildFilterCache()
	}
	return v
}

func (v LibraryView) baseTracks() []library.Track {
	var all []library.Track
	if v.sourceTracks != nil {
		all = v.sourceTracks
	} else {
		all = v.index.Tracks()
	}
	if len(v.tagFilterHashes) == 0 {
		return all
	}
	filtered := make([]library.Track, 0, len(all))
	for _, t := range all {
		if _, ok := v.tagFilterHashes[t.Hash]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// view returns a fully-populated libtui.View from the current LibraryView state.
func (v LibraryView) view() libtui.View {
	lv := libtui.View{
		Index:           v.index,
		Width:           v.width,
		Height:          v.height,
		Cursor:          v.cursor,
		ScrollOffset:    v.scrollOffset,
		Filter:          v.filter,
		FilteredMatches: v.filteredMatches,
		Fuzzy:           v.fuzzy,
		SelectedHashes:  v.selectedHashes,
		ShowDateAdded:   v.showDateAdded,
		ShowPlayedAt:    v.showPlayedAt,
		StripViewLabel:  libraryStripLabel(v.mode),
	}
	switch {
	case v.sortState.Field != SortDefault:
		lv.OverrideTracks = v.sortedTracks
		lv.SortField = int(v.sortState.Field)
		lv.SortDir = int(v.sortState.Dir)
		lv.StripSortLabel = SortFieldLabel(v.sortState.Field)
		lv.StripSortIndicator = SortDirIndicator(v.sortState.Dir)
	case v.sourceTracks != nil:
		lv.OverrideTracks = v.sourceTracks
	case len(v.tagFilterHashes) > 0:
		lv.OverrideTracks = v.baseTracks()
	}
	if SortFieldHasBuckets(v.sortState.Field) {
		field := v.sortState.Field
		now := time.Now()
		lv.BucketLabel = func(t library.Track) string {
			return TrackBucket(field, t, now)
		}
	}
	return lv
}

// cycleSortField advances to the next sort field from the given list, resets
// the cursor, and recomputes the sorted/filtered caches. It returns the
// updated view and the new SortState for notification purposes.
func (v LibraryView) cycleSortField(fields []SortField) (LibraryView, SortState) {
	next := CycleSortField(v.sortState.Field, fields)
	v.sortState.Field = next
	if next == SortDefault {
		v.sortState.Dir = SortAsc
	}
	v = v.refreshView()
	v.cursor = 0
	v.scrollOffset = 0
	return v, v.sortState
}

// applySortDir sets the sort direction, preserving the focused track position.
// Returns (updated view, sort state, true) when applied, or (v, zero, false)
// when the current sort field is SortDefault.
func (v LibraryView) applySortDir(dir SortDir) (LibraryView, SortState, bool) {
	if v.sortState.Field == SortDefault {
		return v, SortState{}, false
	}
	var focusedHash string
	if t, ok := v.view().FocusedTrack(); ok {
		focusedHash = t.Hash
	}
	v.sortState.Dir = dir
	v = v.refreshView()
	v.cursor = v.findTrackPosition(focusedHash)
	v.scrollOffset = adjustScroll(v.cursor, v.scrollOffset, layout.VisibleRows(v.height, libraryViewVisibleReserve(v)))
	return v, v.sortState, true
}

// recomputeSortedTracks rebuilds the sorted tracks cache based on the current
// sort state. When sorting is default, the cache is cleared.
func (v LibraryView) recomputeSortedTracks() LibraryView {
	if v.sortState.Field == SortDefault {
		v.sortedTracks = nil
		return v
	}
	v.sortedTracks = SortTracks(v.baseTracks(), v.sortState.Field, v.sortState.Dir)
	return v
}

// rebuildFilterCache recomputes filteredMatches for the current filterText.
// Call this whenever filterText changes so navigation and rendering reuse the
// cached slice instead of re-running the fuzzy search on every frame.
func (v LibraryView) rebuildFilterCache() LibraryView {
	if v.filter.Text != "" && v.fuzzy != nil {
		tracks := v.baseTracks()
		if v.sortedTracks != nil {
			tracks = v.sortedTracks
		}
		v.filteredMatches = v.fuzzy.Search(v.filter.Text, tracks)
	} else {
		v.filteredMatches = nil
	}
	return v
}

// refreshView recomputes sort and filter caches in one call. Both underlying
// methods are no-ops when sort is default or filter is empty respectively.
func (v LibraryView) refreshView() LibraryView {
	return v.recomputeSortedTracks().rebuildFilterCache()
}

// findTrackPosition returns the cursor position for a track with the given hash
// in the current effective track list (sorted + filtered). Returns 0 if not found.
func (v LibraryView) findTrackPosition(hash string) int {
	if hash == "" {
		return 0
	}
	tracks := v.sortedTracks
	if tracks == nil {
		tracks = v.baseTracks()
	}
	if v.filter.Text != "" && v.filteredMatches != nil {
		for i, m := range v.filteredMatches {
			if m.TrackIndex >= 0 && m.TrackIndex < len(tracks) && tracks[m.TrackIndex].Hash == hash {
				return i
			}
		}
		return 0
	}
	for i, t := range tracks {
		if t.Hash == hash {
			return i
		}
	}
	return 0
}

func (v LibraryView) trackCount() int {
	if v.filter.Text != "" && v.filteredMatches != nil {
		return len(v.filteredMatches)
	}
	if v.sortedTracks != nil {
		return len(v.sortedTracks)
	}
	return len(v.baseTracks())
}

func (v LibraryView) clampCursor() LibraryView {
	count := v.trackCount()
	if count == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return v
	}
	if v.cursor >= count {
		v.cursor = count - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	v.scrollOffset = adjustScroll(v.cursor, v.scrollOffset, layout.VisibleRows(v.height, libraryViewVisibleReserve(v)))
	return v
}
