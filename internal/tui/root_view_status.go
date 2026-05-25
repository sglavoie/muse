package tui

import (
	"fmt"
	"time"
)

// renderStatusBar builds the status bar line shown at the bottom of the view.
func (m Model) renderStatusBar() string {
	sb := m.statusBar
	sb.playbackState = m.playback.state
	sb.repeatMode = m.playback.repeatMode
	sb.shuffle = m.playback.shuffle
	sb.stopAfterCurrent = m.playback.stopAfterCurrent
	sb.visualMode = m.visual.active
	sb.radioMode = m.radio.enabled
	sb.configCreated = m.configCreated
	sb.watchDegraded = m.watchDegraded
	sb.queueEmpty = len(m.queue.entries) == 0
	if m.playback.state == PlaybackPlaying && m.playback.currentIndex >= 0 && m.playback.currentIndex < len(m.queue.entries) {
		sb.queuePosition = fmt.Sprintf("%d of %d", m.playback.currentIndex+1, len(m.queue.entries))
	} else {
		sb.queuePosition = ""
	}
	if m.playback.elapsedSecs > 0 || m.playback.totalSecs > 0 {
		sb.elapsed = formatTime(m.playback.elapsedSecs)
		sb.total = formatTime(m.playback.totalSecs)
	}
	sb.elapsedSecs = m.playback.elapsedSecs
	sb.totalSecs = m.playback.totalSecs
	sb.yankCount = len(m.yankBuffer)
	sb.yankIsCut = m.yankIsCut
	sb.selectionCount = m.activeViewSelectionCount()
	if m.undo.slot.action != nil {
		sb.undoExpiresAt = m.undo.slot.expiresAt
	}
	sb.bucketLabel = m.focusedBucketLabel()
	sb.longOp = m.longOp
	return sb.render(m.termWidth, *m.styles)
}

// activeViewSelectionCount returns the bulk-selection size for the active view.
func (m Model) activeViewSelectionCount() int {
	switch m.activeView {
	case viewLibrary:
		return len(m.library.selectedHashes)
	case viewQueue:
		return len(m.queue.selectedHashes())
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return len(m.browse.artists.selectedHashes)
		case BrowseModeAlbums:
			return len(m.browse.albums.selectedHashes)
		case BrowseModeGenres:
			return len(m.browse.genres.selectedHashes)
		}
	case viewFavorites:
		return len(m.favorites.selectedItems)
	}
	return 0
}

// focusedBucketLabel returns the bucket label for the focused row in the
// active view, or "" when no bucketed sort is in effect.
func (m Model) focusedBucketLabel() string {
	switch m.activeView {
	case viewLibrary:
		field := m.library.sortState.Field
		if !SortFieldHasBuckets(field) {
			return ""
		}
		track, ok := m.library.view().FocusedTrack()
		if !ok {
			return ""
		}
		return TrackBucket(field, track, time.Now())
	case viewQueue:
		field := m.queue.sortState.Field
		if !SortFieldHasBuckets(field) {
			return ""
		}
		entries := m.queue.filteredEntries()
		if m.queue.cursor < 0 || m.queue.cursor >= len(entries) {
			return ""
		}
		return QueueEntryBucket(field, entries[m.queue.cursor])
	default:
		return ""
	}
}

// hasCommittedFilter reports whether the active view has a committed
// (non-empty, non-active) text filter applied.
func (m Model) hasCommittedFilter() bool {
	committed := func(text string, active bool) bool { return text != "" && !active }
	switch m.activeView {
	case viewLibrary:
		return committed(m.library.filter.Text, m.library.filter.Active)
	case viewQueue:
		return committed(m.queue.filter.Text, m.queue.filter.Active)
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return committed(m.browse.artists.filter.Text, m.browse.artists.filter.Active)
		case BrowseModeAlbums:
			return committed(m.browse.albums.filter.Text, m.browse.albums.filter.Active)
		case BrowseModeGenres:
			return committed(m.browse.genres.filter.Text, m.browse.genres.filter.Active)
		}
	case viewFavorites:
		return committed(m.favorites.filter.Text, m.favorites.filter.Active)
	case viewTags:
		return committed(m.tagsView.filter.Text, m.tagsView.filter.Active)
	}
	return false
}
