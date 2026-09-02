package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// QueueEntry holds a single item in the playback queue.
type QueueEntry struct {
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
	origIdx        int    // index in the unfiltered/unsorted entries slice; set by filteredEntries
}

// QueueView renders the playback queue panel.
//
// Key-input routing: all queue actions are dispatched by the root model via
// HandleAction after the keybinding registry resolves the raw key to an action.
// QueueView.Update is therefore a no-op stub kept only so the type satisfies a
// (QueueView, tea.Cmd) call-site contract. Do NOT add key-string handlers to
// Update — that would duplicate registry logic and create a path where registry
// key renames silently stop working.
type QueueView struct {
	entries             []QueueEntry
	cursor              int // index of the currently selected entry
	scrollOffset        int // first visible entry index
	width               int
	height              int
	savingAsPlaylist    bool            // true while the inline save-as-playlist input is active
	savingFromSelection bool            // true when saving only bulk-selected entries (not the full queue)
	saveInput           textinput.Model // current value of the inline save field
	sortState           sorting.SortState
	displayOrder        []int // nil when sort inactive (identity mapping)
	filter              filter.State
	tagFilterHashes     map[string]struct{} // non-nil when a cross-view tag filter is active
}

func (v *QueueView) SetSize(w, h int) { v.width = w; v.height = h }

// recomputeDisplayOrder rebuilds the sorted display index mapping.
func (v QueueView) recomputeDisplayOrder() QueueView {
	v.displayOrder = SortQueueIndices(v.entries, v.sortState.Field, v.sortState.Dir)
	return v
}

// entryIndex translates a display row index to the underlying entries index.
func (v QueueView) entryIndex(displayIdx int) int {
	if v.displayOrder == nil {
		return displayIdx
	}
	if displayIdx < 0 || displayIdx >= len(v.displayOrder) {
		return displayIdx
	}
	return v.displayOrder[displayIdx]
}

// findEntryDisplayPosition returns the display row for the given underlying
// entry index. Returns 0 if not found.
func (v QueueView) findEntryDisplayPosition(entryIdx int) int {
	if v.displayOrder == nil {
		return entryIdx
	}
	for i, idx := range v.displayOrder {
		if idx == entryIdx {
			return i
		}
	}
	return 0
}

// focusedEntryIndex returns the underlying entries index for the focused row,
// accounting for committed filters and tag filters.
func (v QueueView) focusedEntryIndex() int {
	filtered := v.filteredEntries()
	if v.cursor >= 0 && v.cursor < len(filtered) {
		return filtered[v.cursor].origIdx
	}
	return v.entryIndex(v.cursor)
}

// findEntryFilteredPosition returns the filtered display row for the given
// underlying entry index. Returns 0 if the entry is not visible.
func (v QueueView) findEntryFilteredPosition(entryIdx int) int {
	for i, e := range v.filteredEntries() {
		if e.origIdx == entryIdx {
			return i
		}
	}
	return 0
}

// Update is a no-op stub. See the QueueView godoc for why all key handling is
// done via HandleAction instead.
func (v QueueView) Update(_ tea.Msg) (QueueView, tea.Cmd) {
	return v, nil
}
