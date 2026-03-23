package tui

import (
	tea "charm.land/bubbletea/v2"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// handleNavigationAction handles cursor movement, selection toggle,
// and selection clear across all views.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handleNavigationAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionCursorUp, actionCursorDown,
		actionHalfPageDown, actionHalfPageUp,
		actionFullPageDown, actionFullPageUp,
		actionGoToTop, actionGoToBottom:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewArtists:
			m.artists = m.artists.HandleAction(action)
		case viewAlbums:
			m.albums = m.albums.HandleAction(action)
		case viewFavorites:
			m.favorites = m.favorites.HandleAction(action)
		default:
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd, true
		}
		return m, nil, true
	case actionBulkSelectionToggle:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewFavorites:
			if m.favorites.level != favtui.CategoryLevel {
				m.favorites = m.favorites.toggleSelection(m.favorites.cursor)
			}
		default:
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd, true
		}
		return m, nil, true
	case actionClearSelection:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.ClearSelection()
		case viewQueue:
			for i := range m.queue.entries {
				m.queue.entries[i].Selected = false
			}
		case viewPlaylists:
			m.playlists.selectedTrackIDs = nil
		case viewFavorites:
			m.favorites = m.favorites.clearSelection()
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
