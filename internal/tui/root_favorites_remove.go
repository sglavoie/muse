package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// handleFavoritesRemove handles 'F' in the Favorites view (removes the focused item).
func (m Model) handleFavoritesRemove() (Model, tea.Cmd, bool) {
	if m.favoriteWriter == nil {
		return m, nil, true
	}
	writer := m.favoriteWriter

	switch m.favorites.level {
	case favtui.ListLevel:
		switch m.favorites.category {
		case favtui.CategoryArtists:
			name, ok := m.favorites.focusedArtist()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfString(m.favorites.artists, name)
			favLevel := m.favorites.level
			favCategory := m.favorites.category
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
			name, ok := m.favorites.focusedAlbum()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfString(m.favorites.albums, name)
			favLevel := m.favorites.level
			favCategory := m.favorites.category
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
			track, ok := m.favorites.focusedTrack()
			if !ok {
				return m, nil, true
			}
			removedIdx := indexOfTrackByHash(m.favorites.tracks, track.Hash)
			favLevel := m.favorites.level
			favCategory := m.favorites.category
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
		track, ok := m.favorites.focusedTrack()
		if !ok {
			return m, nil, true
		}
		removedIdx := indexOfTrackByHash(m.favorites.drilldownTracks, track.Hash)
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
	if m.favoriteWriter == nil || !m.favorites.hasSelection() {
		return m, nil, true
	}
	writer := m.favoriteWriter

	favLevel := m.favorites.level
	favCategory := m.favorites.category

	switch m.favorites.level {
	case favtui.ListLevel:
		var values []string
		var removedIndices []int
		var removedTracks []library.Track
		var typ string
		switch m.favorites.category {
		case favtui.CategoryArtists:
			typ = "artist"
			items := arttui.FilterStrings(m.favorites.artists, m.favorites.filter.Text)
			for idx := range m.favorites.selectedItems {
				if idx >= 0 && idx < len(items) {
					val := items[idx]
					values = append(values, val)
					for i, a := range m.favorites.artists {
						if a == val {
							removedIndices = append(removedIndices, i)
							break
						}
					}
				}
			}
		case favtui.CategoryAlbums:
			typ = "album"
			items := arttui.FilterStrings(m.favorites.albums, m.favorites.filter.Text)
			for idx := range m.favorites.selectedItems {
				if idx >= 0 && idx < len(items) {
					val := items[idx]
					values = append(values, val)
					for i, a := range m.favorites.albums {
						if a == val {
							removedIndices = append(removedIndices, i)
							break
						}
					}
				}
			}
		case favtui.CategoryTracks:
			typ = "track"
			items := arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
			for idx := range m.favorites.selectedItems {
				if idx >= 0 && idx < len(items) {
					t := items[idx]
					values = append(values, t.Hash)
					removedTracks = append(removedTracks, t)
					for i, tr := range m.favorites.tracks {
						if tr.Hash == t.Hash {
							removedIndices = append(removedIndices, i)
							break
						}
					}
				}
			}
		}
		if len(values) == 0 {
			return m, nil, true
		}
		sortBulkRemovalByIndex(values, removedTracks, removedIndices)
		m.favorites = m.favorites.clearSelection()
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
		items := arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
		var hashes []string
		var removedTracks []library.Track
		var removedIndices []int
		for idx := range m.favorites.selectedItems {
			if idx >= 0 && idx < len(items) {
				t := items[idx]
				hashes = append(hashes, t.Hash)
				removedTracks = append(removedTracks, t)
				for i, tr := range m.favorites.drilldownTracks {
					if tr.Hash == t.Hash {
						removedIndices = append(removedIndices, i)
						break
					}
				}
			}
		}
		if len(hashes) == 0 {
			return m, nil, true
		}
		sortBulkRemovalByIndex(hashes, removedTracks, removedIndices)
		m.favorites = m.favorites.clearSelection()
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
