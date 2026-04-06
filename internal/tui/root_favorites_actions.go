package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// handleFavoriteKeyAction handles favorites-specific key actions.
// Returns (model, cmd, true) when the action was handled.
func (m Model) handleFavoriteKeyAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionFavoritesOpen:
		return m.handleFavoritesOpen()
	case actionFavoritesBack:
		if m.activeView == viewFavorites {
			m.favorites = m.favorites.back()
			return m, nil, true
		}
		return m, nil, false
	case actionToggleFavorite:
		return m.handleToggleFavorite()
	case actionBulkAction:
		if m.activeView == viewFavorites {
			return m.handleFavoritesBulkAction()
		}
		return m, nil, false
	default:
		return m, nil, false
	}
}

// handleToggleFavorite handles the 'F' key across all views.
func (m Model) handleToggleFavorite() (Model, tea.Cmd, bool) {
	if m.favoriteReader == nil || m.favoriteWriter == nil {
		return m, nil, false
	}

	reader := m.favoriteReader
	writer := m.favoriteWriter

	// Determine the favorite type and value(s) based on the active view.
	switch m.activeView {
	case viewLibrary:
		if len(m.library.selectedHashes) > 0 {
			hashes := make([]string, 0, len(m.library.selectedHashes))
			for h := range m.library.selectedHashes {
				hashes = append(hashes, h)
			}
			m.library.selectedHashes = nil
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		if t, ok := m.library.view().FocusedTrack(); ok {
			return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
		}

	case viewRecentlyAdded:
		if len(m.recentlyAdded.selectedHashes) > 0 {
			hashes := make([]string, 0, len(m.recentlyAdded.selectedHashes))
			for h := range m.recentlyAdded.selectedHashes {
				hashes = append(hashes, h)
			}
			m.recentlyAdded.selectedHashes = nil
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		if t, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
		}

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			if name, ok := m.artists.focusedArtist(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
			}
		case arttui.AlbumLevel:
			if name, ok := m.artists.focusedAlbum(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
			}
		case arttui.TrackLevel:
			if t, ok := m.artists.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewAlbums:
		switch m.albums.level {
		case albtui.AlbumLevel:
			if name, ok := m.albums.focusedAlbum(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "album", name), true
			}
		case albtui.TrackLevel:
			if t, ok := m.albums.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewQueue:
		if m.queue.hasSelection() {
			hashes := m.queue.selectedHashes()
			for i := range m.queue.entries {
				m.queue.entries[i].Selected = false
			}
			return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
		}
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			return m, m.toggleFavoriteSingle(reader, writer, "track", e.Hash), true
		}

	case viewPlaylists:
		if m.playlists.level == pltui.TrackLevel {
			if m.playlists.hasTrackSelection() {
				var hashes []string
				for _, t := range m.playlists.tracks {
					if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
						hashes = append(hashes, t.Hash)
					}
				}
				m.playlists.selectedTrackIDs = nil
				return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
			}
			if t, ok := m.playlists.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewGenres:
		switch m.genres.level {
		case gentui.ArtistLevel:
			if name, ok := m.genres.focusedArtist(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "artist", name), true
			}
		case gentui.TrackLevel:
			if len(m.genres.selectedHashes) > 0 {
				hashes := make([]string, 0, len(m.genres.selectedHashes))
				for h := range m.genres.selectedHashes {
					hashes = append(hashes, h)
				}
				m.genres.selectedHashes = nil
				return m, m.toggleFavoriteBulk(reader, writer, "track", hashes), true
			}
			if t, ok := m.genres.focusedTrack(); ok {
				return m, m.toggleFavoriteSingle(reader, writer, "track", t.Hash), true
			}
		}

	case viewFavorites:
		return m.handleFavoritesRemove()
	}

	return m, nil, true
}

// toggleFavoriteSingle returns a Cmd that checks IsFavorite and toggles.
func (m Model) toggleFavoriteSingle(reader favoriteReader, writer favoriteWriter, typ, value string) tea.Cmd {
	return func() tea.Msg {
		isFav, err := reader.IsFavorite(typ, value)
		if err != nil {
			return favoriteToggledMsg{typ: typ, value: value, err: err}
		}
		if isFav {
			err = writer.RemoveFavorite(typ, value)
		} else {
			err = writer.AddFavorite(typ, value)
		}
		msg := favoriteToggledMsg{typ: typ, value: value, added: !isFav, err: err}
		if isFav {
			msg.removedIdx = -1
		}
		return msg
	}
}

// toggleFavoriteBulk returns a Cmd that toggles multiple values. Each value is
// checked individually so that the toggle semantics apply per-item.
func (m Model) toggleFavoriteBulk(reader favoriteReader, writer favoriteWriter, typ string, values []string) tea.Cmd {
	return func() tea.Msg {
		var added, removed int
		var addedValues, removedValues []string
		for _, v := range values {
			isFav, err := reader.IsFavorite(typ, v)
			if err != nil {
				continue
			}
			if isFav {
				if err := writer.RemoveFavorite(typ, v); err != nil {
					slog.Warn("favorite remove failed", "err", err)
				}
				removed++
				removedValues = append(removedValues, v)
			} else {
				if err := writer.AddFavorite(typ, v); err != nil {
					slog.Warn("favorite add failed", "err", err)
				}
				added++
				addedValues = append(addedValues, v)
			}
		}
		msg := favoriteToggledMsg{
			typ:        typ,
			count:      added + removed,
			added:      added > 0,
			removedIdx: -1,
		}
		if added == 0 && len(removedValues) > 0 {
			msg.removedValues = removedValues
		}
		if removed == 0 && len(addedValues) > 0 {
			msg.addedValues = addedValues
		}
		return msg
	}
}

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

// handleFavoritesOpen handles the enter key in the Favorites view.
func (m Model) handleFavoritesOpen() (Model, tea.Cmd, bool) {
	if m.activeView != viewFavorites {
		return m, nil, false
	}

	switch m.favorites.level {
	case favtui.CategoryLevel:
		if m.favoriteReader == nil {
			return m, nil, true
		}
		category := favtui.Category(m.favorites.cursor)
		reader := m.favoriteReader
		libraryTracks := m.library.index.Tracks()
		return m, func() tea.Msg {
			switch category {
			case favtui.CategoryArtists:
				rows, err := reader.ReadFavoritesByType("artist")
				if err != nil {
					return favoritesLoadedMsg{category: int(category), err: err}
				}
				artists := make([]string, len(rows))
				for i, r := range rows {
					artists[i] = r.Value
				}
				return favoritesLoadedMsg{category: int(category), artists: artists}
			case favtui.CategoryAlbums:
				rows, err := reader.ReadFavoritesByType("album")
				if err != nil {
					return favoritesLoadedMsg{category: int(category), err: err}
				}
				albums := make([]string, len(rows))
				for i, r := range rows {
					albums[i] = r.Value
				}
				return favoritesLoadedMsg{category: int(category), albums: albums}
			case favtui.CategoryTracks:
				rows, err := reader.ReadFavoritesByType("track")
				if err != nil {
					return favoritesLoadedMsg{category: int(category), err: err}
				}
				hashSet := make(map[string]struct{}, len(rows))
				for _, r := range rows {
					hashSet[r.Value] = struct{}{}
				}
				var tracks []library.Track
				for _, t := range libraryTracks {
					if _, ok := hashSet[t.Hash]; ok {
						tracks = append(tracks, t)
						delete(hashSet, t.Hash)
					}
				}
				return favoritesLoadedMsg{category: int(category), tracks: tracks}
			default:
				return favoritesLoadedMsg{category: int(category)}
			}
		}, true

	case favtui.ListLevel:
		if m.favorites.category == favtui.CategoryTracks {
			if m.queueWriter != nil {
				if cmd := m.favorites.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
					return m, cmd, true
				}
			}
			return m, nil, true
		}
		tracks := m.library.index.Tracks()
		switch m.favorites.category {
		case favtui.CategoryArtists:
			name, ok := m.favorites.focusedArtist()
			if !ok {
				return m, nil, true
			}
			derived := deduplicateTracksByHash(arttui.DeriveArtistTracks(tracks, name))
			m.favorites = m.favorites.openTracks(derived, name)
		case favtui.CategoryAlbums:
			name, ok := m.favorites.focusedAlbum()
			if !ok {
				return m, nil, true
			}
			derived := deduplicateTracksByHash(albtui.DeriveAlbumTracks(tracks, name))
			m.favorites = m.favorites.openTracks(derived, name)
		}
		return m, nil, true

	case favtui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.favorites.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
		return m, nil, true
	}

	return m, nil, true
}
