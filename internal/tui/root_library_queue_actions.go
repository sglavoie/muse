package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
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
		cmd = m.library.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
		m.library = m.library.ClearFilter()
	case viewMixes:
		if m.queueWriter != nil && m.mixes.Level == 1 {
			cmd = m.mixes.TrackView.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
		}
	case viewInsights:
		if m.queueWriter != nil && m.insights.AtTracks() {
			cmd = m.insights.TrackView.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
		}
	}
	return m, cmd, true
}

func (m Model) handleLibraryPlayNextAction() (tea.Model, tea.Cmd, bool) {
	if m.queueWriter == nil {
		return m, nil, true
	}

	insertIdx := m.playNextIndex()
	prevLen := len(m.queue.Entries)
	var cmd tea.Cmd
	switch m.activeView {
	case viewLibrary:
		cmd = m.library.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
		m.library = m.library.ClearFilter()
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				cmd = m.browse.Artists.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				cmd = m.browse.Albums.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		case browse.BrowseModeGenres:
			if _, ok := m.browse.Genres.FocusedTrack(); ok {
				cmd = m.browse.Genres.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
			}
		}
	case viewPlaylists:
		if m.playlists.Level == pltui.TrackLevel {
			cmd = m.playlists.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewFavorites:
		if _, ok := m.favorites.FocusedTrack(); ok {
			cmd = m.favorites.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
		}
	case viewTags:
		if _, ok := m.tagsView.FocusedTrack(); ok {
			cmd = m.tagsView.PlayNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
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
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				m = m.clearQueueForReplace()
				cmd = m.browse.Artists.EnqueueFocusedTrack(m.queueWriter, 0)
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				m = m.clearQueueForReplace()
				cmd = m.browse.Albums.EnqueueFocusedTrack(m.queueWriter, 0)
			}
		case browse.BrowseModeGenres:
			if _, ok := m.browse.Genres.FocusedTrack(); ok {
				m = m.clearQueueForReplace()
				cmd = m.browse.Genres.EnqueueFocusedTrack(m.queueWriter, 0)
			}
		}
	case viewPlaylists:
		if m.playlists.Level == pltui.TrackLevel {
			m = m.clearQueueForReplace()
			cmd = m.playlists.EnqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewFavorites:
		if _, ok := m.favorites.FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.favorites.EnqueueFocusedTrack(m.queueWriter, 0)
		}
	case viewTags:
		if _, ok := m.tagsView.FocusedTrack(); ok {
			m = m.clearQueueForReplace()
			cmd = m.tagsView.EnqueueFocusedTrack(m.queueWriter, 0)
		}
	}
	return m, cmd, true
}
