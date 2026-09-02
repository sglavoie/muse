package queue

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

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
}

// View renders the playback queue panel.
//
// Key-input routing: all queue actions are dispatched by the root model via
// HandleAction after the keybinding registry resolves the raw key to an action.
// View.Update is therefore a no-op stub kept only so the type satisfies a
// (View, tea.Cmd) call-site contract. Do NOT add key-string handlers to
// Update — that would duplicate registry logic and create a path where registry
// key renames silently stop working.
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
	DisplayOrder        []int // nil when sort inactive (identity mapping)
	Filter              filter.State
	TagFilterHashes     map[string]struct{} // non-nil when a cross-view tag filter is active
}

func (v *View) SetSize(w, h int) { v.Width = w; v.Height = h }

// RecomputeDisplayOrder rebuilds the sorted display index mapping.
func (v View) RecomputeDisplayOrder() View {
	v.DisplayOrder = SortQueueIndices(v.Entries, v.SortState.Field, v.SortState.Dir)
	return v
}

// EntryIndex translates a display row index to the underlying entries index.
func (v View) EntryIndex(displayIdx int) int {
	if v.DisplayOrder == nil {
		return displayIdx
	}
	if displayIdx < 0 || displayIdx >= len(v.DisplayOrder) {
		return displayIdx
	}
	return v.DisplayOrder[displayIdx]
}

// FindEntryDisplayPosition returns the display row for the given underlying
// entry index. Returns 0 if not found.
func (v View) FindEntryDisplayPosition(entryIdx int) int {
	if v.DisplayOrder == nil {
		return entryIdx
	}
	for i, idx := range v.DisplayOrder {
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

// Update is a no-op stub. See the View godoc for why all key handling is
// done via HandleAction instead.
func (v View) Update(_ tea.Msg) (View, tea.Cmd) {
	return v, nil
}
