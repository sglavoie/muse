package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// LibraryMode identifies the sub-mode of the Library view.
type LibraryMode uint8

const (
	LibraryModeAll     LibraryMode = iota // standard Library (all tracks, default sort)
	LibraryModeRecent                     // Recently Added (sorted by date added)
	LibraryModeHistory                    // History (sorted by last played)
)

// libraryModeSnapshot saves per-mode cursor/scroll/filter state for restoration
// when the user cycles back to a previously visited mode.
type libraryModeSnapshot struct {
	cursor int
	scroll int
	filter filter.State
}

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index           library.LibraryIndex
	sourceTracks    []library.Track
	width           int
	height          int
	cursor          int // zero-based index of the focused track row
	scrollOffset    int // first visible track index
	filter          filter.State
	filteredMatches []library.Match // cached result of the last fuzzy search
	fuzzy           library.FuzzySearcher
	selectedHashes  map[string]struct{} // bulk-selected track hashes
	sortState       SortState
	sortedTracks    []library.Track // cached sorted copy; nil when SortDefault
	showDateAdded   bool
	showPlayedAt    bool
	tagFilterHashes map[string]struct{} // non-nil when a cross-view tag filter is active
	mode            LibraryMode
	modeState       [3]libraryModeSnapshot // saved cursor/scroll/filter per mode
}

func (v *LibraryView) setSize(w, h int) { v.width = w; v.height = h }

// render returns a string representation of the library panel with proportional
// columnar layout. Column widths resize with the terminal width.
func (v LibraryView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	lv := v.view()
	lv.Styles = s
	lv.FavoriteHashes = favHashes
	lv.FavoriteArtists = favArtists
	lv.FavoriteAlbums = favAlbums
	lv.NowPlayingHash = nowPlayingHash
	lv.NowPlayingIcon = nowPlayingIcon
	return lv.Render()
}

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
		// sourceTracks is set by syncRecentlyAddedFromLibrary after this call
		v.sortState = SortState{Field: SortDateAdded, Dir: SortDesc}
		v.sortedTracks = nil
	case LibraryModeHistory:
		v.showDateAdded = false
		v.showPlayedAt = true
		// sourceTracks is loaded by the caller via historyReader
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
		// When a tag filter is active but no sort/source override exists,
		// explicitly pass the filtered track list so the inner View uses
		// the correct (smaller) set for navigation and rendering.
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

// ToggleBulkSelection toggles hash in the selected set. If hash is empty it is
// a no-op. The map is lazily initialised on first toggle.
func (v LibraryView) ToggleBulkSelection(hash string) LibraryView {
	if hash == "" {
		return v
	}
	if v.selectedHashes == nil {
		v.selectedHashes = make(map[string]struct{})
	}
	if _, ok := v.selectedHashes[hash]; ok {
		delete(v.selectedHashes, hash)
	} else {
		v.selectedHashes[hash] = struct{}{}
	}
	return v
}

// ClearFilter deactivates the filter bar, resets cursor/scroll, and rebuilds
// the filter cache. It is a no-op when no filter text is active.
func (v LibraryView) ClearFilter() LibraryView {
	if v.filter.Text == "" {
		return v
	}
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	v = v.rebuildFilterCache()
	return v
}

// ClearSelection removes all bulk-selection markers.
func (v LibraryView) ClearSelection() LibraryView {
	v.selectedHashes = nil
	return v
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
	reserve := 1
	if v.filter.Active || v.filter.Text != "" {
		reserve++
	}
	v.scrollOffset = adjustScroll(v.cursor, v.scrollOffset, layout.VisibleRows(v.height, reserve))
	return v
}

// syncRecentlyAddedFromLibrary refreshes the library view's source tracks when
// the Library is in Recent mode. It is a no-op in other modes (data is loaded
// lazily when the user switches to that mode).
func (m Model) syncRecentlyAddedFromLibrary() Model {
	if m.library.mode != LibraryModeRecent {
		return m
	}
	m.library.sourceTracks = m.library.index.Tracks()
	if m.library.fuzzy == nil {
		m.library.fuzzy = library.SahilmFuzzySearcher{}
	}
	if m.library.sortState.Field == SortDefault {
		m.library.sortState = SortState{Field: SortDateAdded, Dir: SortDesc}
	}
	m.library = m.library.refreshView().clampCursor()
	return m
}

// HandleAction executes a pre-resolved library action dispatched by root.Update.
func (v LibraryView) HandleAction(a action) LibraryView {
	view := v.view()
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
	case actionBulkSelectionToggle:
		if t, ok := view.FocusedTrack(); ok {
			return v.ToggleBulkSelection(t.Hash)
		}
		return v
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. It uses the full filter state stored on LibraryView so
// that the correct filtered-result track is selected when a filter is active.
// Returns nil when the library is empty or the cursor is out of range.
func (v LibraryView) EnqueueFocused(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	_, cmd := v.view().EnqueueFocused(qa, prevQueueLen)
	return cmd
}

// PlayNextFocused dispatches a tea.Cmd that inserts the focused track right
// after the currently playing track. insertAfter is the queue index to insert
// at. Returns nil when the library is empty or the cursor is out of range.
func (v LibraryView) PlayNextFocused(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	_, cmd := v.view().PlayNextFocused(qa, prevQueueLen, insertAfter)
	return cmd
}

// EnqueueSelected dispatches a tea.Cmd that appends all bulk-selected tracks to
// the play queue via qa, in index order, then clears the selection. Returns the
// updated view (with selection cleared) and a nil Cmd when no tracks are
// selected. prevQueueLen is used to set WasEmpty on the first enqueued track so
// the root model can trigger immediate playback when the queue was empty.
func (v LibraryView) EnqueueSelected(qa libtui.QueueAppender, prevQueueLen int) (LibraryView, tea.Cmd) {
	if len(v.selectedHashes) == 0 {
		return v, nil
	}
	var cmds []tea.Cmd
	queueLen := prevQueueLen
	tracks := v.baseTracks()
	if v.sortedTracks != nil {
		tracks = v.sortedTracks
	}
	for _, t := range tracks {
		if _, ok := v.selectedHashes[t.Hash]; !ok {
			continue
		}
		track := t
		wasEmpty := queueLen == 0
		queueLen++
		cmds = append(cmds, func() tea.Msg {
			id, err := qa.AppendTrack(track, "")
			return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, InsertAfter: -1}
		})
	}
	v = v.ClearSelection()
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// libraryStripLabel returns the view-label string shown in the state
// strip for the given library mode. The label tracks the title bar
// abbreviation so users see a consistent view name in both places.
func libraryStripLabel(mode LibraryMode) string {
	switch mode {
	case LibraryModeRecent:
		return "Recent"
	case LibraryModeHistory:
		return "History"
	default:
		return "Library"
	}
}

// formatCount formats n with comma separators (e.g. 1234 → "1,234").
func formatCount(n int) string {
	// Format via %d so math.MinInt prints correctly; then operate on the
	// digit string directly. The obvious `formatCount(-n)` recursion would
	// loop forever when n == math.MinInt because -math.MinInt overflows
	// back to math.MinInt.
	raw := fmt.Sprintf("%d", n)
	sign := ""
	digits := raw
	if len(raw) > 0 && raw[0] == '-' {
		sign = "-"
		digits = raw[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	offset := len(digits) % 3
	var result []byte
	for i := range len(digits) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digits[i])
	}
	return sign + string(result)
}
