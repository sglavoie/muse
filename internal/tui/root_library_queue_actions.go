package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
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
	if !m.atEnqueueTrackLevel() {
		return m, nil, true
	}
	var cmd tea.Cmd
	switch m.ActiveView {
	case viewLibrary:
		if m.queueWriter == nil {
			return m, nil, true
		}
		cmd = m.library.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
	case viewMixes:
		if m.queueWriter != nil && m.mixes.Level == 1 {
			cmd = m.mixes.TrackView.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
		}
	case viewPlaylists:
		return m.handlePlaylistOpen()
	case viewFavorites:
		return m.handleFavoritesOpen()
	case viewTags:
		next, cmd := m.handleTagOpen()
		return next, cmd, true
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return m.handleArtistOpenAction()
		case browse.BrowseModeAlbums:
			return m.handleAlbumOpenAction()
		case browse.BrowseModeGenres:
			return m.handleGenreOpenAction()
		}
	case viewInsights:
		if m.queueWriter != nil && m.insights.AtTracks() {
			cmd = m.insights.TrackView.EnqueueFocused(m.queueWriter, len(m.queue.Entries))
		}
	}
	return m, cmd, true
}

func (m Model) handleLibraryPlayNextAction() (tea.Model, tea.Cmd, bool) {
	if m.queueWriter == nil || !m.atEnqueueTrackLevel() {
		return m, nil, true
	}
	tracks, source := m.queueActionTracks()
	if len(tracks) == 0 {
		return m, nil, true
	}
	return m, libtui.EnqueueTrackCmd(tracks[0], true, m.queueWriter, len(m.queue.Entries), source, m.playNextIndex()), true
}

func (m Model) handleLibraryEnqueueReplaceAction() (tea.Model, tea.Cmd, bool) {
	return m.requestQueueReplacement()
}
