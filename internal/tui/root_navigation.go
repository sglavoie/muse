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
		case viewBrowse:
			switch m.browse.mode {
			case BrowseModeArtists:
				m.browse.artists = m.browse.artists.HandleAction(action)
			case BrowseModeAlbums:
				m.browse.albums = m.browse.albums.HandleAction(action)
			case BrowseModeGenres:
				m.browse.genres = m.browse.genres.HandleAction(action)
			}
		case viewFavorites:
			m.favorites = m.favorites.HandleAction(action)
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
		case viewBrowse:
			switch m.browse.mode {
			case BrowseModeArtists:
				if m.browse.artists.level == arttui.TrackLevel {
					if t, ok := m.browse.artists.focusedTrack(); ok {
						m.browse.artists.selectedHashes = toggleHashInSet(m.browse.artists.selectedHashes, t.Hash)
					}
				}
			case BrowseModeAlbums:
				if m.browse.albums.level == albtui.TrackLevel {
					if t, ok := m.browse.albums.focusedTrack(); ok {
						m.browse.albums.selectedHashes = toggleHashInSet(m.browse.albums.selectedHashes, t.Hash)
					}
				}
			case BrowseModeGenres:
				if m.browse.genres.level == gentui.TrackLevel {
					if t, ok := m.browse.genres.focusedTrack(); ok {
						m.browse.genres.selectedHashes = toggleHashInSet(m.browse.genres.selectedHashes, t.Hash)
					}
				}
			}
		case viewTags:
			if m.tagsView.level == tagtui.TrackLevel {
				if t, ok := m.tagsView.focusedTrack(); ok {
					m.tagsView.selectedHashes = toggleHashInSet(m.tagsView.selectedHashes, t.Hash)
				}
			}
		case viewMixes:
			if m.mixes.level == 1 {
				if t := m.mixes.focusedTrack(); t != nil {
					m.mixes.trackView.selectedHashes = toggleHashInSet(m.mixes.trackView.selectedHashes, t.Hash)
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
		case viewBrowse:
			switch m.browse.mode {
			case BrowseModeArtists:
				m.browse.artists.selectedHashes = nil
			case BrowseModeAlbums:
				m.browse.albums.selectedHashes = nil
			case BrowseModeGenres:
				m.browse.genres.selectedHashes = nil
			}
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

// toggleHashInSet lazily initialises sel when nil, then toggles hash in it:
// removes hash if present, adds it otherwise. Returns the updated map.
func toggleHashInSet(sel map[string]struct{}, hash string) map[string]struct{} {
	if sel == nil {
		sel = make(map[string]struct{})
	}
	if _, ok := sel[hash]; ok {
		delete(sel, hash)
	} else {
		sel[hash] = struct{}{}
	}
	return sel
}
