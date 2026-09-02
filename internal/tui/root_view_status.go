package tui

import (
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
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
	sb.QueueEmpty = len(m.queue.Entries) == 0
	if m.playback.state == playback.StatePlaying && m.playback.currentIndex >= 0 && m.playback.currentIndex < len(m.queue.Entries) {
		sb.QueuePosition = fmt.Sprintf("%d of %d", m.playback.currentIndex+1, len(m.queue.Entries))
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
		return len(m.library.SelectedHashes)
	case viewQueue:
		return len(m.queue.SelectedHashes())
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return len(m.browse.Artists.SelectedHashes)
		case browse.BrowseModeAlbums:
			return len(m.browse.Albums.SelectedHashes)
		case browse.BrowseModeGenres:
			return len(m.browse.Genres.SelectedHashes)
		}
	case viewFavorites:
		return len(m.favorites.SelectedItems)
	case viewInsights:
		return len(m.insights.trackView.SelectedHashes)
	}
	return 0
}

// focusedBucketLabel returns the bucket label for the focused row in the
// active view, or "" when no bucketed sort is in effect.
func (m Model) focusedBucketLabel() string {
	switch m.activeView {
	case viewLibrary:
		field := m.library.SortState.Field
		if !sorting.SortFieldHasBuckets(field) {
			return ""
		}
		track, ok := m.library.Panel().FocusedTrack()
		if !ok {
			return ""
		}
		return sorting.TrackBucket(field, track, time.Now())
	case viewQueue:
		field := m.queue.SortState.Field
		if !sorting.SortFieldHasBuckets(field) {
			return ""
		}
		entries := m.queue.FilteredEntries()
		if m.queue.Cursor < 0 || m.queue.Cursor >= len(entries) {
			return ""
		}
		return queue.EntryBucket(field, entries[m.queue.Cursor])
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
		return committed(m.library.Filter.Text, m.library.Filter.Active)
	case viewQueue:
		return committed(m.queue.Filter.Text, m.queue.Filter.Active)
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return committed(m.browse.Artists.Filter.Text, m.browse.Artists.Filter.Active)
		case browse.BrowseModeAlbums:
			return committed(m.browse.Albums.Filter.Text, m.browse.Albums.Filter.Active)
		case browse.BrowseModeGenres:
			return committed(m.browse.Genres.Filter.Text, m.browse.Genres.Filter.Active)
		}
	case viewFavorites:
		return committed(m.favorites.Filter.Text, m.favorites.Filter.Active)
	case viewTags:
		return committed(m.tagsView.Filter.Text, m.tagsView.Filter.Active)
	case viewInsights:
		return committed(m.insights.trackView.Filter.Text, m.insights.trackView.Filter.Active)
	}
	return false
}
