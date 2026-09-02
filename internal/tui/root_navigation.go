package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleNavigationAction handles cursor movement, selection toggle,
// and selection clear across all views.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handleNavigationAction(action keys.Action) (Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionVisualMode:
		if m.visual.active {
			m = exitVisualMode(m)
		} else {
			m = enterVisualMode(m)
		}
		return m, nil, true
	case keys.ActionCursorUp, keys.ActionCursorDown,
		keys.ActionHalfPageDown, keys.ActionHalfPageUp,
		keys.ActionFullPageDown, keys.ActionFullPageUp,
		keys.ActionGoToTop, keys.ActionGoToBottom:
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
		case viewInsights:
			m.insights = m.insights.handleCursorAction(action)
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
	case keys.ActionBulkSelectionToggle:
		// Exit visual mode first: while active the next cursor move restores
		// the pre-visual snapshot and re-applies the range, which silently
		// throws away any toggle issued inside the range. Treating the toggle
		// as committing the current visual range matches the vim convention.
		if m.visual.active {
			m = exitVisualMode(m)
		}
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewFavorites:
			if m.favorites.Level != favtui.CategoryLevel {
				m.favorites = m.favorites.ToggleSelection(m.favorites.Cursor)
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
			if m.tagsView.Level == tagtui.TrackLevel {
				if t, ok := m.tagsView.FocusedTrack(); ok {
					m.tagsView.SelectedHashes = toggleHashInSet(m.tagsView.SelectedHashes, t.Hash)
				}
			}
		case viewMixes:
			if m.mixes.level == 1 {
				if t := m.mixes.focusedTrack(); t != nil {
					m.mixes.trackView.SelectedHashes = toggleHashInSet(m.mixes.trackView.SelectedHashes, t.Hash)
				}
			}
		case viewInsights:
			if m.insights.atTracks() {
				if t := m.insights.focusedTrack(); t != nil {
					m.insights.trackView.SelectedHashes = toggleHashInSet(m.insights.trackView.SelectedHashes, t.Hash)
				}
			}
		default:
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd, true
		}
		return m, nil, true
	case keys.ActionClearSelection:
		m = exitVisualMode(m)
		m = m.clearActiveViewSelection()
		return m, nil, true
	default:
		return m, nil, false
	}
}

// clearActiveViewSelection removes all bulk-selection markers in the currently
// active view. It does not touch other views or exit visual mode.
func (m Model) clearActiveViewSelection() Model {
	switch m.activeView {
	case viewLibrary:
		m.library = m.library.ClearSelection()
	case viewQueue:
		for i := range m.queue.entries {
			m.queue.entries[i].Selected = false
		}
	case viewPlaylists:
		m.playlists.SelectedTrackIDs = nil
	case viewFavorites:
		m.favorites = m.favorites.ClearSelection()
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
		m.tagsView.SelectedHashes = nil
	case viewMixes:
		m.mixes.trackView.SelectedHashes = nil
	case viewInsights:
		m.insights.trackView.SelectedHashes = nil
	}
	return m
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
