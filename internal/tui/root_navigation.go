package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
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
		case viewArtists:
			if m.artists.level == arttui.TrackLevel {
				if t, ok := m.artists.focusedTrack(); ok {
					if m.artists.selectedHashes == nil {
						m.artists.selectedHashes = make(map[string]struct{})
					}
					if _, sel := m.artists.selectedHashes[t.Hash]; sel {
						delete(m.artists.selectedHashes, t.Hash)
					} else {
						m.artists.selectedHashes[t.Hash] = struct{}{}
					}
				}
			}
		case viewAlbums:
			if m.albums.level == albtui.TrackLevel {
				if t, ok := m.albums.focusedTrack(); ok {
					if m.albums.selectedHashes == nil {
						m.albums.selectedHashes = make(map[string]struct{})
					}
					if _, sel := m.albums.selectedHashes[t.Hash]; sel {
						delete(m.albums.selectedHashes, t.Hash)
					} else {
						m.albums.selectedHashes[t.Hash] = struct{}{}
					}
				}
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
		case viewArtists:
			m.artists.selectedHashes = nil
		case viewAlbums:
			m.albums.selectedHashes = nil
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
