package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
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
		switch m.ActiveView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewBrowse:
			switch m.browse.Mode {
			case browse.BrowseModeArtists:
				m.browse.Artists = m.browse.Artists.HandleAction(action)
			case browse.BrowseModeAlbums:
				m.browse.Albums = m.browse.Albums.HandleAction(action)
			case browse.BrowseModeGenres:
				m.browse.Genres = m.browse.Genres.HandleAction(action)
			}
		case viewFavorites:
			m.favorites = m.favorites.HandleAction(action)
		case viewTags:
			m.tagsView = m.tagsView.HandleAction(action)
		case viewMixes:
			m.mixes = m.mixes.HandleCursorAction(action)
		case viewInsights:
			m.insights = m.insights.HandleCursorAction(action)
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
		switch m.ActiveView {
		case viewLibrary:
			m.library = m.library.HandleAction(action)
		case viewPlaylists:
			m.playlists = m.playlists.HandleAction(action)
		case viewFavorites:
			if m.favorites.Level != favtui.CategoryLevel {
				m.favorites = m.favorites.ToggleSelection(m.favorites.Cursor)
			}
		case viewBrowse:
			switch m.browse.Mode {
			case browse.BrowseModeArtists:
				if m.browse.Artists.Level == arttui.TrackLevel {
					if t, ok := m.browse.Artists.FocusedTrack(); ok {
						m.browse.Artists.SelectedHashes = toggleHashInSet(m.browse.Artists.SelectedHashes, t.Hash)
					}
				}
			case browse.BrowseModeAlbums:
				if m.browse.Albums.Level == albtui.TrackLevel {
					if t, ok := m.browse.Albums.FocusedTrack(); ok {
						m.browse.Albums.SelectedHashes = toggleHashInSet(m.browse.Albums.SelectedHashes, t.Hash)
					}
				}
			case browse.BrowseModeGenres:
				if m.browse.Genres.Level == gentui.TrackLevel {
					if t, ok := m.browse.Genres.FocusedTrack(); ok {
						m.browse.Genres.SelectedHashes = toggleHashInSet(m.browse.Genres.SelectedHashes, t.Hash)
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
			if m.mixes.Level == 1 {
				if t := m.mixes.FocusedTrack(); t != nil {
					m.mixes.TrackView.SelectedHashes = toggleHashInSet(m.mixes.TrackView.SelectedHashes, t.Hash)
				}
			}
		case viewInsights:
			if m.insights.AtTracks() {
				if t := m.insights.FocusedTrack(); t != nil {
					m.insights.TrackView.SelectedHashes = toggleHashInSet(m.insights.TrackView.SelectedHashes, t.Hash)
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
	switch m.ActiveView {
	case viewLibrary:
		m.library = m.library.ClearSelection()
	case viewQueue:
		for i := range m.queue.Entries {
			m.queue.Entries[i].Selected = false
		}
	case viewPlaylists:
		m.playlists.SelectedTrackIDs = nil
	case viewFavorites:
		m.favorites = m.favorites.ClearSelection()
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			m.browse.Artists.SelectedHashes = nil
		case browse.BrowseModeAlbums:
			m.browse.Albums.SelectedHashes = nil
		case browse.BrowseModeGenres:
			m.browse.Genres.SelectedHashes = nil
		}
	case viewTags:
		m.tagsView.SelectedHashes = nil
	case viewMixes:
		m.mixes.TrackView.SelectedHashes = nil
	case viewInsights:
		m.insights.TrackView.SelectedHashes = nil
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
