package queue

import (
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// Entry holds a single item in the playback queue.
type Entry struct {
	ID             int64  // database row ID; 0 when not persisted
	Path           string // file path used to identify the entry
	Hash           string // BLAKE3 content hash; used to rebuild player.QueueTrack on undo
	Display        string // human-readable label, e.g. "Artist – Title"
	Missing        bool   // true when the file was not found at playback time
	Artist         string // track artist, used by the status bar persistent left zone
	Title          string // track title, used by the status bar persistent left zone
	Album          string // track album name
	Selected       bool   // true when the entry is part of a bulk selection
	DurationMs     int64  // track duration in milliseconds from the database
	Position       int64  // gapped-integer queue position from the database
	SourcePlaylist string // playlist name this entry was enqueued from; empty for individual enqueues
	TrackNumber    int    // track number from audio metadata; 0 = unknown
	DiscNumber     int    // disc number from audio metadata; 0 = unknown
	OrigIdx        int    // index in the unfiltered/unsorted entries slice; set by filteredEntries

	// DJOwned reports that the Auto-DJ chose this row and may replace it when
	// the user applies new settings. It mirrors the stored provenance flag and
	// is never inferred from SourcePlaylist, which is a display label: a
	// playlist the user named "Radio" and every row written before migration
	// 024 read as false and stay protected from a refresh.
	DJOwned bool

	// DJReasons is the planner's explanation for a DJ selection, empty for
	// manual entries. Display data only; nothing decides ownership from it.
	DJReasons radio.Reasons
}

// View renders the playback queue panel.
//
// Key-input routing: all queue actions are dispatched by the root model via
// HandleAction after the keybinding registry resolves the raw key to an action.
// The view deliberately has no bubbletea-style Update method. Do NOT add one
// that handles key strings — that would duplicate registry logic and create a
// path where registry key renames silently stop working.
type View struct {
	Entries             []Entry
	Cursor              int // index of the currently selected entry
	ScrollOffset        int // first visible entry index
	Width               int
	Height              int
	SavingAsPlaylist    bool            // true while the inline save-as-playlist input is active
	SavingFromSelection bool            // true when saving only bulk-selected entries (not the full queue)
	SaveInput           textinput.Model // current value of the inline save field
	SortState           sorting.SortState
	DisplayOrder        []int // nil when no explicit metadata sort is active
	Filter              filter.State
	TagFilterHashes     map[string]struct{} // non-nil when a cross-view tag filter is active
	// Traversal reads the player-owned playback timeline. Nil when the player
	// exposes none, in which case rows keep persisted queue order.
	Traversal TraversalSource
}

func (v *View) SetSize(w, h int) { v.Width = w; v.Height = h }

// RecomputeDisplayOrder rebuilds the sorted display index mapping.
func (v View) RecomputeDisplayOrder() View {
	v.DisplayOrder = SortQueueIndices(v.Entries, v.SortState.Field, v.SortState.Dir)
	return v
}

// effectiveOrder resolves the order rows are displayed in, given one timeline
// snapshot. An explicit metadata sort wins, because the user asked for it; the
// live shuffled traversal comes next, because it is what will actually play;
// persisted queue order (nil, meaning identity) is the floor. An explicit sort
// that no longer lines up with the queue falls straight to persisted order
// rather than to the traversal, so the panel never disagrees with the "sort"
// chunk the state strip is showing.
func (v View) effectiveOrder(t Traversal) []int {
	n := len(v.Entries)
	if v.DisplayOrder != nil {
		if validDisplayOrder(v.DisplayOrder, n) {
			return v.DisplayOrder
		}
		return nil
	}
	order, ok := t.order(n)
	if !ok {
		return nil
	}
	return order
}

// timelineActive reports whether rows are currently ordered by the live
// shuffled traversal rather than by persisted queue order. Position-based
// edits must refuse while it is true: a display row then names a different
// queue row than its position suggests, which is what PlacementBlockReason
// exposes.
func (v View) timelineActive(t Traversal) bool {
	if v.DisplayOrder != nil {
		return false
	}
	_, ok := t.order(len(v.Entries))
	return ok
}

// EntryIndex translates a display row index to the underlying entries index.
func (v View) EntryIndex(displayIdx int) int {
	order := v.effectiveOrder(v.traversalSnapshot())
	if order == nil || displayIdx < 0 || displayIdx >= len(order) {
		return displayIdx
	}
	return order[displayIdx]
}

// FindEntryDisplayPosition returns the display row for the given underlying
// entry index. Returns 0 if not found.
func (v View) FindEntryDisplayPosition(entryIdx int) int {
	order := v.effectiveOrder(v.traversalSnapshot())
	if order == nil {
		return entryIdx
	}
	for i, idx := range order {
		if idx == entryIdx {
			return i
		}
	}
	return 0
}

// FocusedEntryIndex returns the underlying entries index for the focused row,
// accounting for committed filters and tag filters.
func (v View) FocusedEntryIndex() int {
	filtered := v.FilteredEntries()
	if v.Cursor >= 0 && v.Cursor < len(filtered) {
		return filtered[v.Cursor].OrigIdx
	}
	return v.EntryIndex(v.Cursor)
}

// FindEntryFilteredPosition returns the filtered display row for the given
// underlying entry index. Returns 0 if the entry is not visible.
func (v View) FindEntryFilteredPosition(entryIdx int) int {
	for i, e := range v.FilteredEntries() {
		if e.OrigIdx == entryIdx {
			return i
		}
	}
	return 0
}
