package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func (m Model) handleLibraryQueueAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionLibraryEnqueue:
		return m.handleLibraryEnqueueAction()
	case actionPlayNext:
		return m.handleLibraryPlayNextAction()
	case actionLibraryEnqueueReplace:
		return m.handleLibraryEnqueueReplaceAction()
	default:
		return m, nil, false
	}
}

func (m Model) handleLibraryEnqueueAction() (tea.Model, tea.Cmd, bool) {
	if m.queueWriter == nil {
		return m, nil, true
	}

	var cmd tea.Cmd
	switch m.activeView {
	case viewLibrary:
		cmd = m.library.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		m.library = m.library.ClearFilter()
	case viewRecentlyAdded:
		cmd = m.recentlyAdded.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		m.recentlyAdded = m.recentlyAdded.ClearFilter()
	case viewHistory:
		cmd = m.history.EnqueueFocused(m.queueWriter, len(m.queue.entries))
		m.history = m.history.ClearFilter()
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
	case viewRecentlyAdded:
		cmd = m.recentlyAdded.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
		m.recentlyAdded = m.recentlyAdded.ClearFilter()
	case viewHistory:
		cmd = m.history.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
		m.history = m.history.ClearFilter()
	case viewArtists:
		if m.artists.level == arttui.TrackLevel {
			cmd = m.artists.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewAlbums:
		if m.albums.level == albtui.TrackLevel {
			cmd = m.albums.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewPlaylists:
		if m.playlists.level == pltui.TrackLevel {
			cmd = m.playlists.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewFavorites:
		if _, ok := m.favorites.focusedTrack(); ok {
			cmd = m.favorites.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewGenres:
		if _, ok := m.genres.focusedTrack(); ok {
			cmd = m.genres.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
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
		if _, ok := m.library.view().FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.library.EnqueueFocused(m.queueWriter, 0)
			m.library = m.library.ClearFilter()
		}
	case viewRecentlyAdded:
		if _, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.recentlyAdded.EnqueueFocused(m.queueWriter, 0)
			m.recentlyAdded = m.recentlyAdded.ClearFilter()
		}
	case viewHistory:
		if _, ok := m.history.view().FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.history.EnqueueFocused(m.queueWriter, 0)
			m.history = m.history.ClearFilter()
		}
	case viewArtists:
		if m.artists.level == arttui.TrackLevel {
			m = m.clearQueueForReplace()
			cmd = m.artists.enqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewAlbums:
		if m.albums.level == albtui.TrackLevel {
			m = m.clearQueueForReplace()
			cmd = m.albums.enqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewPlaylists:
		if m.playlists.level == pltui.TrackLevel {
			m = m.clearQueueForReplace()
			cmd = m.playlists.enqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewFavorites:
		if _, ok := m.favorites.focusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.favorites.enqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewGenres:
		if _, ok := m.genres.focusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.genres.enqueueFocusedTrack(m.queueWriter, 0)
		}
	}
	return m, cmd, true
}
