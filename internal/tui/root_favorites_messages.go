package tui

import (
	"fmt"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
)

// handleFavoriteToggledMsg processes the result of a favorite toggle operation.
func (m Model) handleFavoriteToggledMsg(msg favoriteToggledMsg) Model {
	if msg.err != nil {
		m.statusBar.notify("Toggle favorite failed: " + msg.err.Error())
		return m
	}

	var note string
	switch {
	case msg.count > 1 && msg.added:
		note = fmt.Sprintf("Added %d %ss to favorites", msg.count, msg.typ)
	case msg.count > 1:
		note = fmt.Sprintf("Removed %d %ss from favorites", msg.count, msg.typ)
	case msg.added:
		note = fmt.Sprintf("Added %s to favorites", msg.typ)
	default:
		note = fmt.Sprintf("Removed %s from favorites", msg.typ)
	}

	switch {
	case !msg.added && msg.count > 1 && len(msg.removedValues) > 0:
		m.setUndoSlot(undoBulkRemoveFavoritesAction{
			typ:      msg.typ,
			values:   msg.removedValues,
			tracks:   msg.removedTracks,
			indices:  msg.removedIndices,
			level:    msg.favLevel,
			category: msg.favCategory,
		}, nil)
	case !msg.added:
		m.setUndoSlot(undoRemoveFavoriteAction{
			typ:      msg.typ,
			value:    msg.value,
			track:    msg.removedTrack,
			idx:      msg.removedIdx,
			level:    msg.favLevel,
			category: msg.favCategory,
		}, nil)
	case len(msg.addedValues) > 0:
		m.setUndoSlot(undoBulkAddFavoritesAction{
			typ:    msg.typ,
			values: msg.addedValues,
		}, nil)
	case msg.added && msg.value != "":
		m.setUndoSlot(undoAddFavoriteAction{
			typ:   msg.typ,
			value: msg.value,
		}, nil)
	default:
		m.statusBar.notify(note)
	}

	switch msg.typ {
	case "track":
		updateFavoriteCache(&m.favoriteHashes, m.favoriteReader, "track", msg)
	case "artist":
		updateFavoriteCache(&m.favoriteArtists, m.favoriteReader, "artist", msg)
	case "album":
		updateFavoriteCache(&m.favoriteAlbums, m.favoriteReader, "album", msg)
	}

	if m.activeView == viewFavorites && !msg.added {
		valuesToRemove := msg.removedValues
		if len(valuesToRemove) == 0 && msg.value != "" {
			valuesToRemove = []string{msg.value}
		}
		switch m.favorites.level {
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				for _, v := range valuesToRemove {
					m.favorites.artists = removeString(m.favorites.artists, v)
				}
				m.favorites.artistCount = len(m.favorites.artists)
			case favtui.CategoryAlbums:
				for _, v := range valuesToRemove {
					m.favorites.albums = removeString(m.favorites.albums, v)
				}
				m.favorites.albumCount = len(m.favorites.albums)
			case favtui.CategoryTracks:
				for _, v := range valuesToRemove {
					m.favorites.tracks = removeTrackByHash(m.favorites.tracks, v)
				}
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			m.favorites.selectedItems = nil
			m.favorites = m.favorites.clampCursor()
		case favtui.TrackLevel:
			for _, v := range valuesToRemove {
				m.favorites.drilldownTracks = removeTrackByHash(m.favorites.drilldownTracks, v)
			}
			m.favorites.selectedItems = nil
			m.favorites = m.favorites.clampCursor()
		}
	}

	if m.activeView == viewFavorites && m.favoriteReader != nil {
		a, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = a
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
	}

	return m
}

// handleFavoritesLoadedMsg processes the result of loading favorites for a category.
func (m Model) handleFavoritesLoadedMsg(msg favoritesLoadedMsg) Model {
	if msg.err != nil {
		m.statusBar.notify("Load favorites failed: " + msg.err.Error())
		return m
	}

	category := favtui.Category(msg.category)
	m.favorites.category = category
	switch category {
	case favtui.CategoryArtists:
		m.favorites.artists = msg.artists
	case favtui.CategoryAlbums:
		m.favorites.albums = msg.albums
	case favtui.CategoryTracks:
		m.favorites.tracks = msg.tracks
	}
	m.favorites.categoryCursor = m.favorites.cursor
	m.favorites.level = favtui.ListLevel
	m.favorites.cursor = 0
	m.favorites.scrollOffset = 0
	m.favorites.filter = filter.State{}
	m.favorites.sortState = SortState{}
	m.favorites.selectedItems = nil
	return m
}
