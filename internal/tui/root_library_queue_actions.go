package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func (m Model) handleLibraryQueueAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionLibraryEnqueue:
		return m.handleLibraryEnqueueAction()
	case keys.ActionPlayNext:
		return m.handleLibraryPlayNextAction()
	case keys.ActionLibraryEnqueueReplace:
		return m.handleLibraryEnqueueReplaceAction()
	default:
		return m, nil, false
	}
}

func (m Model) handleLibraryEnqueueAction() (tea.Model, tea.Cmd, bool) {
	var cmd tea.Cmd
	switch m.activeView {
	case viewLibrary:
		if len(m.library.SelectedHashes) > 0 {
			if m.batchEnqueuer == nil {
				m.statusBar.Notify("Enqueue not available")
				return m, nil, true
			}
			tracks := m.bulkLibraryTracks()
			if len(tracks) == 0 {
				m.statusBar.Notify("No selected tracks available")
				return m, nil, true
			}
			cmd = m.batchEnqueueTracksCmd(tracks, "", "Queue")
			m = exitVisualMode(m)
			m.library = m.library.ClearSelection()
			m.library = m.library.ClearFilter()
			return m, cmd, true
		}
		if m.queueWriter == nil {
			return m, nil, true
		}
		cmd = m.library.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		m.library = m.library.ClearFilter()
	case viewMixes:
		if m.queueWriter != nil && m.mixes.level == 1 {
			cmd = m.mixes.trackView.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		}
	case viewInsights:
		if m.queueWriter != nil && m.insights.atTracks() {
			cmd = m.insights.trackView.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		}
	}
	return m, cmd, true
}

func (m Model) handleLibraryPlayNextAction() (tea.Model, tea.Cmd, bool) {
	if m.queueWriter == nil {
		return m, nil, true
	}

	insertIdx := m.playNextIndex()
	prevLen := len(m.queue.entries)
	var cmd tea.Cmd
	switch m.activeView {
	case viewLibrary:
		cmd = m.library.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
		m.library = m.library.ClearFilter()
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.level == arttui.TrackLevel {
				cmd = m.browse.artists.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		case BrowseModeAlbums:
			if m.browse.albums.level == albtui.TrackLevel {
				cmd = m.browse.albums.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		case BrowseModeGenres:
			if _, ok := m.browse.genres.focusedTrack(); ok {
				cmd = m.browse.genres.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		}
	case viewPlaylists:
		if m.playlists.Level == pltui.TrackLevel {
			cmd = m.playlists.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewFavorites:
		if _, ok := m.favorites.focusedTrack(); ok {
			cmd = m.favorites.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewTags:
		if _, ok := m.tagsView.focusedTrack(); ok {
			cmd = m.tagsView.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	}
	return m, cmd, true
}

func (m Model) handleLibraryEnqueueReplaceAction() (tea.Model, tea.Cmd, bool) {
	if m.queueWriter == nil {
		return m, nil, true
	}

	var cmd tea.Cmd
	switch m.activeView {
	case viewLibrary:
		if _, ok := m.library.Panel().FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.library.EnqueueFocused(m.queueWriter, 0)
			m.library = m.library.ClearFilter()
		}
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.level == arttui.TrackLevel {
				m = m.clearQueueForReplace()
				cmd = m.browse.artists.enqueueFocusedTrack(m.queueWriter, 0)
			}
		case BrowseModeAlbums:
			if m.browse.albums.level == albtui.TrackLevel {
				m = m.clearQueueForReplace()
				cmd = m.browse.albums.enqueueFocusedTrack(m.queueWriter, 0)
			}
		case BrowseModeGenres:
			if _, ok := m.browse.genres.focusedTrack(); ok {
				m = m.clearQueueForReplace()
				cmd = m.browse.genres.enqueueFocusedTrack(m.queueWriter, 0)
			}
		}
	case viewPlaylists:
		if m.playlists.Level == pltui.TrackLevel {
			m = m.clearQueueForReplace()
			cmd = m.playlists.EnqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewFavorites:
		if _, ok := m.favorites.focusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.favorites.enqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewTags:
		if _, ok := m.tagsView.focusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.tagsView.enqueueFocusedTrack(m.queueWriter, 0)
		}
	}
	return m, cmd, true
}
