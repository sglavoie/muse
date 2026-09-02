package tui

import (
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/text"
)

// renderStatusBar builds the status bar line shown at the bottom of the view.
func (m Model) renderStatusBar() string {
	sb := m.statusBar
	sb.PlaybackState = m.playback.state
	sb.RepeatMode = m.playback.repeatMode
	sb.Shuffle = m.playback.shuffle
	sb.StopAfterCurrent = m.playback.stopAfterCurrent
	sb.VisualMode = m.visual.active
	sb.RadioMode = m.radio.enabled
	sb.ConfigCreated = m.configCreated
	sb.WatchDegraded = m.watchDegraded
	sb.QueueEmpty = len(m.queue.entries) == 0
	if m.playback.state == playback.StatePlaying && m.playback.currentIndex >= 0 && m.playback.currentIndex < len(m.queue.entries) {
		sb.QueuePosition = fmt.Sprintf("%d of %d", m.playback.currentIndex+1, len(m.queue.entries))
	} else {
		sb.QueuePosition = ""
	}
	if m.playback.elapsedSecs > 0 || m.playback.totalSecs > 0 {
		sb.Elapsed = text.FormatPositionSecs(m.playback.elapsedSecs)
		sb.Total = text.FormatPositionSecs(m.playback.totalSecs)
	}
	sb.ElapsedSecs = m.playback.elapsedSecs
	sb.TotalSecs = m.playback.totalSecs
	sb.YankCount = len(m.yankBuffer)
	sb.YankIsCut = m.yankIsCut
	sb.SelectionCount = m.activeViewSelectionCount()
	if m.undo.slot.action != nil {
		sb.UndoExpiresAt = m.undo.slot.expiresAt
	}
	sb.BucketLabel = m.focusedBucketLabel()
	sb.LongOp = m.longOp
	return sb.Render(m.termWidth, *m.styles)
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
	case viewInsights:
		return len(m.insights.trackView.selectedHashes)
	}
	return 0
}

// focusedBucketLabel returns the bucket label for the focused row in the
// active view, or "" when no bucketed sort is in effect.
func (m Model) focusedBucketLabel() string {
	switch m.activeView {
	case viewLibrary:
		field := m.library.sortState.Field
		if !sorting.SortFieldHasBuckets(field) {
			return ""
		}
		track, ok := m.library.view().FocusedTrack()
		if !ok {
			return ""
		}
		return sorting.TrackBucket(field, track, time.Now())
	case viewQueue:
		field := m.queue.sortState.Field
		if !sorting.SortFieldHasBuckets(field) {
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
	case viewInsights:
		return committed(m.insights.trackView.filter.Text, m.insights.trackView.filter.Active)
	}
	return false
}
