package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// handleFavoritesOpen handles the enter key in the Favorites view.
func (m Model) handleFavoritesOpen() (Model, tea.Cmd, bool) {
	if m.activeView != viewFavorites {
		return m, nil, false
	}

	switch m.favorites.Level {
	case favtui.CategoryLevel:
		if m.favoriteReader == nil {
			return m, nil, true
		}
		category := favtui.Category(m.favorites.Cursor)
		reader := m.favoriteReader
		libraryTracks := snapshotTracks(m.library.Index.Tracks())
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
		if m.favorites.Category == favtui.CategoryTracks {
			if m.queueWriter != nil {
				if cmd := m.favorites.EnqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
					return m, cmd, true
				}
			}
			return m, nil, true
		}
		tracks := m.library.Index.Tracks()
		switch m.favorites.Category {
		case favtui.CategoryArtists:
			name, ok := m.favorites.FocusedArtist()
			if !ok {
				return m, nil, true
			}
			derived := deduplicateTracksByHash(arttui.DeriveArtistTracks(tracks, name))
			m.favorites = m.favorites.OpenTracks(derived, name)
		case favtui.CategoryAlbums:
			name, ok := m.favorites.FocusedAlbum()
			if !ok {
				return m, nil, true
			}
			derived := deduplicateTracksByHash(albtui.DeriveAlbumTracks(tracks, name))
			m.favorites = m.favorites.OpenTracks(derived, name)
		}
		return m, nil, true

	case favtui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.favorites.EnqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
		return m, nil, true
	}

	return m, nil, true
}
