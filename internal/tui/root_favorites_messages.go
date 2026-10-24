package tui

import (
	"fmt"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// handleFavoriteToggledMsg processes the result of a favorite toggle operation.
func (m Model) handleFavoriteToggledMsg(msg favoriteToggledMsg) Model {
	if msg.err != nil {
		m.statusBar.NotifyError("Toggle favorite failed: " + msg.err.Error())
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
		m.statusBar.Notify(note)
	}

	switch msg.typ {
	case "track":
		updateFavoriteCache(&m.favoriteHashes, m.favoriteReader, "track", msg)
		// Favorited tracks carry a scoring bonus, so the set is an Auto-DJ
		// input. Artist and album favorites are not read by the planner.
		m.djInvalidate()
	case "artist":
		updateFavoriteCache(&m.favoriteArtists, m.favoriteReader, "artist", msg)
	case "album":
		updateFavoriteCache(&m.favoriteAlbums, m.favoriteReader, "album", msg)
	}

	applyToFavoritesView := !msg.fromTargetPicker || msg.pickerOriginView == viewFavorites
	if m.ActiveView == viewFavorites && !msg.added && applyToFavoritesView {
		valuesToRemove := msg.removedValues
		if len(valuesToRemove) == 0 && msg.value != "" {
			valuesToRemove = []string{msg.value}
		}
		switch m.favorites.Level {
		case favtui.ListLevel:
			switch m.favorites.Category {
			case favtui.CategoryArtists:
				for _, v := range valuesToRemove {
					m.favorites.Artists = removeString(m.favorites.Artists, v)
				}
				m.favorites.ArtistCount = len(m.favorites.Artists)
			case favtui.CategoryAlbums:
				for _, v := range valuesToRemove {
					m.favorites.Albums = removeString(m.favorites.Albums, v)
				}
				m.favorites.AlbumCount = len(m.favorites.Albums)
			case favtui.CategoryTracks:
				for _, v := range valuesToRemove {
					m.favorites.Tracks = removeTrackByHash(m.favorites.Tracks, v)
				}
				m.favorites.TrackCount = len(m.favorites.Tracks)
			}
			m.favorites.SelectedItems = nil
			m.favorites = m.favorites.ClampCursor()
		case favtui.TrackLevel:
			removedParent := msg.fromTargetPicker && msg.pickerOriginLevel == favtui.TrackLevel &&
				msg.pickerDrilldownName == m.favorites.DrilldownName &&
				containsString(valuesToRemove, m.favorites.DrilldownName)
			switch {
			case removedParent && msg.typ == "artist" && m.favorites.Category == favtui.CategoryArtists:
				m.favorites.Artists = removeString(m.favorites.Artists, m.favorites.DrilldownName)
				m.favorites.ArtistCount = len(m.favorites.Artists)
				m.favorites = m.favorites.Back()
			case removedParent && msg.typ == "album" && m.favorites.Category == favtui.CategoryAlbums:
				m.favorites.Albums = removeString(m.favorites.Albums, m.favorites.DrilldownName)
				m.favorites.AlbumCount = len(m.favorites.Albums)
				m.favorites = m.favorites.Back()
			default:
				m.favorites.SelectedItems = nil
				m.favorites = m.favorites.ClampCursor()
			}
		}
	}

	if m.ActiveView == viewFavorites && m.favoriteReader != nil {
		a, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.ArtistCount = a
			m.favorites.AlbumCount = al
			m.favorites.TrackCount = t
		}
	}

	return m
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// handleFavoritesLoadedMsg processes the result of loading favorites for a category.
// A result for a superseded request (a second Enter before the first load
// landed) or one arriving after the view left CategoryLevel is dropped, so it
// cannot reset the list cursor and filter or save the wrong category cursor.
func (m Model) handleFavoritesLoadedMsg(msg favoritesLoadedMsg) Model {
	if msg.token != 0 && msg.token != m.favorites.PendingLoad {
		return m
	}
	m.favorites.PendingLoad = 0
	if msg.err != nil {
		m.statusBar.NotifyError("Load favorites failed: " + msg.err.Error())
		return m
	}
	if m.favorites.Level != favtui.CategoryLevel {
		return m
	}

	category := favtui.Category(msg.category)
	m.favorites.Category = category
	switch category {
	case favtui.CategoryArtists:
		m.favorites.Artists = msg.artists
	case favtui.CategoryAlbums:
		m.favorites.Albums = msg.albums
	case favtui.CategoryTracks:
		m.favorites.Tracks = msg.tracks
	}
	m.favorites.CategoryCursor = int(category) // the category requested, not where the cursor drifted
	m.favorites.Level = favtui.ListLevel
	m.favorites.Cursor = 0
	m.favorites.ScrollOffset = 0
	m.favorites.Filter = filter.State{}
	m.favorites.SortState = sorting.SortState{}
	m.favorites.SelectedItems = nil
	return m
}
