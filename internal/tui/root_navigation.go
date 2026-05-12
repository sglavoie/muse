package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleNavigationAction handles cursor movement, selection toggle,
// and selection clear across all views.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handleNavigationAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionVisualMode:
		if m.visual.active {
			m = exitVisualMode(m)
		} else {
			m = enterVisualMode(m)
		}
		return m, nil, true
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
		case viewGenres:
			m.genres = m.genres.HandleAction(action)
		case viewTags:
			m.tagsView = m.tagsView.HandleAction(action)
		case viewMixes:
			m.mixes = m.mixes.handleCursorAction(action)
		default:
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			if m.visual.active {
				m = updateVisualSelection(m)
			}
			return m, cmd, true
		}
		if m.visual.active {
			m = updateVisualSelection(m)
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
		case viewGenres:
			if m.genres.level == gentui.TrackLevel {
				if t, ok := m.genres.focusedTrack(); ok {
					if m.genres.selectedHashes == nil {
						m.genres.selectedHashes = make(map[string]struct{})
					}
					if _, sel := m.genres.selectedHashes[t.Hash]; sel {
						delete(m.genres.selectedHashes, t.Hash)
					} else {
						m.genres.selectedHashes[t.Hash] = struct{}{}
					}
				}
			}
		case viewTags:
			if m.tagsView.level == tagtui.TrackLevel {
				if t, ok := m.tagsView.focusedTrack(); ok {
					if m.tagsView.selectedHashes == nil {
						m.tagsView.selectedHashes = make(map[string]struct{})
					}
					if _, sel := m.tagsView.selectedHashes[t.Hash]; sel {
						delete(m.tagsView.selectedHashes, t.Hash)
					} else {
						m.tagsView.selectedHashes[t.Hash] = struct{}{}
					}
				}
			}
		case viewMixes:
			if m.mixes.level == 1 {
				if t := m.mixes.focusedTrack(); t != nil {
					if m.mixes.trackView.selectedHashes == nil {
						m.mixes.trackView.selectedHashes = make(map[string]struct{})
					}
					if _, sel := m.mixes.trackView.selectedHashes[t.Hash]; sel {
						delete(m.mixes.trackView.selectedHashes, t.Hash)
					} else {
						m.mixes.trackView.selectedHashes[t.Hash] = struct{}{}
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
		m = exitVisualMode(m)
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
		case viewGenres:
			m.genres.selectedHashes = nil
		case viewTags:
			m.tagsView.selectedHashes = nil
		case viewMixes:
			m.mixes.trackView.selectedHashes = nil
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
