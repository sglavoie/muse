package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// handleFavoritesRemove handles 'F' in the Favorites view (removes the focused item).
func (m Model) handleFavoritesRemove() (Model, tea.Cmd, bool) {
	if m.favoriteWriter == nil {
		return m, nil, true
	}
	writer := m.favoriteWriter

	switch m.favorites.Level {
	case favtui.ListLevel:
		switch m.favorites.Category {
		case favtui.CategoryArtists:
			name, ok := m.favorites.FocusedArtist()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfString(m.favorites.Artists, name)
			favLevel := m.favorites.Level
			favCategory := m.favorites.Category
			return m, func() tea.Msg {
				err := writer.RemoveFavorite("artist", name)
				return favoriteToggledMsg{
					typ:         "artist",
					value:       name,
					added:       false,
					err:         err,
					removedIdx:  removedIdx,
					favLevel:    favLevel,
					favCategory: favCategory,
				}
			}, true
		case favtui.CategoryAlbums:
			name, ok := m.favorites.FocusedAlbum()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfString(m.favorites.Albums, name)
			favLevel := m.favorites.Level
			favCategory := m.favorites.Category
			return m, func() tea.Msg {
				err := writer.RemoveFavorite("album", name)
				return favoriteToggledMsg{
					typ:         "album",
					value:       name,
					added:       false,
					err:         err,
					removedIdx:  removedIdx,
					favLevel:    favLevel,
					favCategory: favCategory,
				}
			}, true
		case favtui.CategoryTracks:
			track, ok := m.favorites.FocusedTrack()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfTrackByHash(m.favorites.Tracks, track.Hash)
			favLevel := m.favorites.Level
			favCategory := m.favorites.Category
			return m, func() tea.Msg {
				err := writer.RemoveFavorite("track", track.Hash)
				return favoriteToggledMsg{
					typ:          "track",
					value:        track.Hash,
					added:        false,
					err:          err,
					removedIdx:   removedIdx,
					removedTrack: track,
					favLevel:     favLevel,
					favCategory:  favCategory,
				}
			}, true
		}

	case favtui.TrackLevel:
		track, ok := m.favorites.FocusedTrack()
		if !ok {
			return m, nil, true
		}
		removedIdx := indexOfTrackByHash(m.favorites.DrilldownTracks, track.Hash)
		return m, func() tea.Msg {
			err := writer.RemoveFavorite("track", track.Hash)
			return favoriteToggledMsg{
				typ:          "track",
				value:        track.Hash,
				added:        false,
				err:          err,
				removedIdx:   removedIdx,
				removedTrack: track,
				favLevel:     favtui.TrackLevel,
			}
		}, true
	}

	return m, nil, true
}

// handleFavoritesBulkAction handles 'a' in the Favorites view with selected items.
func (m Model) handleFavoritesBulkAction() (Model, tea.Cmd, bool) {
	if m.favoriteWriter == nil || !m.favorites.HasSelection() {
		return m, nil, true
	}
	writer := m.favoriteWriter

	favLevel := m.favorites.Level
	favCategory := m.favorites.Category

	switch m.favorites.Level {
	case favtui.ListLevel:
		var values []string
		var removedIndices []int
		var removedTracks []library.Track
		var typ string
		switch m.favorites.Category {
		case favtui.CategoryArtists:
			typ = "artist"
			for i, value := range m.favorites.Artists {
				if _, ok := m.favorites.SelectedItems[value]; ok {
					values = append(values, value)
					removedIndices = append(removedIndices, i)
				}
			}
		case favtui.CategoryAlbums:
			typ = "album"
			for i, value := range m.favorites.Albums {
				if _, ok := m.favorites.SelectedItems[value]; ok {
					values = append(values, value)
					removedIndices = append(removedIndices, i)
				}
			}
		case favtui.CategoryTracks:
			typ = "track"
			for _, track := range m.favorites.SelectedTracks() {
				values = append(values, track.Hash)
				removedTracks = append(removedTracks, track)
				removedIndices = append(removedIndices, indexOfTrackByHash(m.favorites.Tracks, track.Hash))
			}
		}
		if len(values) == 0 {
			return m, nil, true
		}
		sortBulkRemovalByIndex(values, removedTracks, removedIndices)
		m = exitVisualMode(m)
		m.favorites = m.favorites.ClearSelection()
		return m, func() tea.Msg {
			err := writer.RemoveFavoritesByTypeAndValues(typ, values)
			return favoriteToggledMsg{
				typ:            typ,
				count:          len(values),
				added:          false,
				err:            err,
				removedValues:  values,
				removedTracks:  removedTracks,
				removedIndices: removedIndices,
				favLevel:       favLevel,
				favCategory:    favCategory,
			}
		}, true

	case favtui.TrackLevel:
		var hashes []string
		var removedTracks []library.Track
		var removedIndices []int
		for _, track := range m.favorites.SelectedTracks() {
			hashes = append(hashes, track.Hash)
			removedTracks = append(removedTracks, track)
			removedIndices = append(removedIndices, indexOfTrackByHash(m.favorites.DrilldownTracks, track.Hash))
		}
		if len(hashes) == 0 {
			return m, nil, true
		}
		sortBulkRemovalByIndex(hashes, removedTracks, removedIndices)
		m = exitVisualMode(m)
		m.favorites = m.favorites.ClearSelection()
		return m, func() tea.Msg {
			err := writer.RemoveFavoritesByTypeAndValues("track", hashes)
			return favoriteToggledMsg{
				typ:            "track",
				count:          len(hashes),
				added:          false,
				err:            err,
				removedValues:  hashes,
				removedTracks:  removedTracks,
				removedIndices: removedIndices,
				favLevel:       favLevel,
				favCategory:    favCategory,
			}
		}, true
	}

	return m, nil, true
}
