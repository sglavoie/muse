package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// undoRemoveFavoriteAction reverts removal of a single favorite
// (undoKindRemoveFavorite).
type undoRemoveFavoriteAction struct {
	typ      string
	value    string
	track    library.Track
	idx      int
	level    favtui.Level
	category favtui.Category
}

func (a undoRemoveFavoriteAction) kind() undoKind { return undoKindRemoveFavorite }

func (a undoRemoveFavoriteAction) description() string {
	return fmt.Sprintf("Removed %s from favorites — press u to undo", a.typ)
}

func (a undoRemoveFavoriteAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.AddFavorite(a.typ, a.value); err != nil {
			slog.Warn("undo remove favorite failed", "err", err)
		}
	}
	if a.typ == "track" && a.value != "" {
		if m.favoriteHashes == nil {
			m.favoriteHashes = make(map[string]struct{})
		}
		m.favoriteHashes[a.value] = struct{}{}
	}
	if a.typ == "artist" && a.value != "" {
		if m.favoriteArtists == nil {
			m.favoriteArtists = make(map[string]struct{})
		}
		m.favoriteArtists[a.value] = struct{}{}
	}
	if a.typ == "album" && a.value != "" {
		if m.favoriteAlbums == nil {
			m.favoriteAlbums = make(map[string]struct{})
		}
		m.favoriteAlbums[a.value] = struct{}{}
	}
	if m.activeView == viewFavorites && a.idx >= 0 &&
		m.favorites.level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				m.favorites.artists = insertStringAt(m.favorites.artists, a.idx, a.value)
				m.favorites.artistCount = len(m.favorites.artists)
			case favtui.CategoryAlbums:
				m.favorites.albums = insertStringAt(m.favorites.albums, a.idx, a.value)
				m.favorites.albumCount = len(m.favorites.albums)
			case favtui.CategoryTracks:
				m.favorites.tracks = insertTrackAt(m.favorites.tracks, a.idx, a.track)
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			if m.favorites.category == a.category {
				m.favorites.cursor = a.idx
				m.favorites = m.favorites.clampCursor()
			}
		case favtui.TrackLevel:
			hash := a.value
			if hash == "" {
				hash = a.track.Hash
			}
			if hash != "" && indexOfTrackByHash(m.favorites.drilldownTracks, hash) < 0 && a.track.Hash != "" {
				m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, a.idx, a.track)
			}
			m.favorites.selectedItems = nil
			if a.idx >= 0 {
				m.favorites.cursor = a.idx
			}
			m.favorites = m.favorites.clampCursor()
		}
	}
	if m.activeView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = ac
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
	}
	m.statusBar.Notify(fmt.Sprintf("Undone: added %s back to favorites", a.typ))
	return m, nil
}

// undoAddFavoriteAction reverts a single favorite addition (undoKindAddFavorite).
type undoAddFavoriteAction struct {
	typ   string
	value string
}

func (a undoAddFavoriteAction) kind() undoKind { return undoKindAddFavorite }

func (a undoAddFavoriteAction) description() string {
	return fmt.Sprintf("Added %s to favorites — press u to undo", a.typ)
}

func (a undoAddFavoriteAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavorite(a.typ, a.value); err != nil {
			slog.Warn("undo add favorite failed", "err", err)
		}
	}
	if a.typ == "track" && a.value != "" {
		delete(m.favoriteHashes, a.value)
	}
	if a.typ == "artist" && a.value != "" {
		delete(m.favoriteArtists, a.value)
	}
	if a.typ == "album" && a.value != "" {
		delete(m.favoriteAlbums, a.value)
	}
	if m.activeView == viewFavorites {
		switch m.favorites.level {
		case favtui.ListLevel:
			switch {
			case a.typ == "artist" && m.favorites.category == favtui.CategoryArtists:
				m.favorites.artists = removeString(m.favorites.artists, a.value)
				m.favorites.artistCount = len(m.favorites.artists)
			case a.typ == "album" && m.favorites.category == favtui.CategoryAlbums:
				m.favorites.albums = removeString(m.favorites.albums, a.value)
				m.favorites.albumCount = len(m.favorites.albums)
			case a.typ == "track" && m.favorites.category == favtui.CategoryTracks:
				m.favorites.tracks = removeTrackByHash(m.favorites.tracks, a.value)
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			m.favorites.selectedItems = nil
			m.favorites = m.favorites.clampCursor()
		case favtui.TrackLevel:
			if a.typ == "track" {
				m.favorites.selectedItems = nil
				m.favorites = m.favorites.clampCursor()
			}
		}
		if m.favoriteReader != nil {
			ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.artistCount = ac
				m.favorites.albumCount = al
				m.favorites.trackCount = t
			}
		}
	}
	m.statusBar.Notify(fmt.Sprintf("Undone: removed %s from favorites", a.typ))
	return m, nil
}

// undoBulkAddFavoritesAction reverts a bulk favorite addition
// (undoKindBulkAddFavorites).
type undoBulkAddFavoritesAction struct {
	typ    string
	values []string
}

func (a undoBulkAddFavoritesAction) kind() undoKind { return undoKindBulkAddFavorites }

func (a undoBulkAddFavoritesAction) description() string {
	n := len(a.values)
	return fmt.Sprintf("Added %d %ss to favorites — press u to undo", n, a.typ)
}

func (a undoBulkAddFavoritesAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavoritesByTypeAndValues(a.typ, a.values); err != nil {
			slog.Warn("undo bulk add favorites failed", "err", err)
		}
	}
	if a.typ == "track" {
		for _, v := range a.values {
			delete(m.favoriteHashes, v)
		}
	}
	if a.typ == "artist" {
		for _, v := range a.values {
			delete(m.favoriteArtists, v)
		}
	}
	if a.typ == "album" {
		for _, v := range a.values {
			delete(m.favoriteAlbums, v)
		}
	}
	if m.activeView == viewFavorites {
		switch m.favorites.level {
		case favtui.ListLevel:
			switch {
			case a.typ == "artist" && m.favorites.category == favtui.CategoryArtists:
				for _, v := range a.values {
					m.favorites.artists = removeString(m.favorites.artists, v)
				}
				m.favorites.artistCount = len(m.favorites.artists)
			case a.typ == "album" && m.favorites.category == favtui.CategoryAlbums:
				for _, v := range a.values {
					m.favorites.albums = removeString(m.favorites.albums, v)
				}
				m.favorites.albumCount = len(m.favorites.albums)
			case a.typ == "track" && m.favorites.category == favtui.CategoryTracks:
				for _, v := range a.values {
					m.favorites.tracks = removeTrackByHash(m.favorites.tracks, v)
				}
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			m.favorites.selectedItems = nil
			m.favorites = m.favorites.clampCursor()
		case favtui.TrackLevel:
			if a.typ == "track" {
				m.favorites.selectedItems = nil
				m.favorites = m.favorites.clampCursor()
			}
		}
		if m.favoriteReader != nil {
			ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.artistCount = ac
				m.favorites.albumCount = al
				m.favorites.trackCount = t
			}
		}
	}
	n := len(a.values)
	m.statusBar.Notify(fmt.Sprintf("Undone: removed %d %s(s) from favorites", n, a.typ))
	return m, nil
}

// undoBulkRemoveFavoritesAction reverts bulk removal of favorites
// (undoKindBulkRemoveFavorites).
type undoBulkRemoveFavoritesAction struct {
	typ      string
	values   []string
	tracks   []library.Track
	indices  []int
	level    favtui.Level
	category favtui.Category
}

func (a undoBulkRemoveFavoritesAction) kind() undoKind { return undoKindBulkRemoveFavorites }

func (a undoBulkRemoveFavoritesAction) description() string {
	n := len(a.values)
	return fmt.Sprintf("Removed %d %ss from favorites — press u to undo", n, a.typ)
}

func (a undoBulkRemoveFavoritesAction) perform(m Model) (Model, tea.Cmd) {
	if m.favoriteWriter != nil {
		for _, v := range a.values {
			if err := m.favoriteWriter.AddFavorite(a.typ, v); err != nil {
				slog.Warn("undo bulk remove favorite failed", "err", err)
			}
		}
	}
	if a.typ == "track" {
		if m.favoriteHashes == nil {
			m.favoriteHashes = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteHashes[v] = struct{}{}
		}
	}
	if a.typ == "artist" {
		if m.favoriteArtists == nil {
			m.favoriteArtists = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteArtists[v] = struct{}{}
		}
	}
	if a.typ == "album" {
		if m.favoriteAlbums == nil {
			m.favoriteAlbums = make(map[string]struct{})
		}
		for _, v := range a.values {
			m.favoriteAlbums[v] = struct{}{}
		}
	}
	if m.activeView == viewFavorites && len(a.indices) > 0 &&
		m.favorites.level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				for i, idx := range a.indices {
					m.favorites.artists = insertStringAt(m.favorites.artists, min(idx, len(m.favorites.artists)), a.values[i])
				}
				m.favorites.artistCount = len(m.favorites.artists)
			case favtui.CategoryAlbums:
				for i, idx := range a.indices {
					m.favorites.albums = insertStringAt(m.favorites.albums, min(idx, len(m.favorites.albums)), a.values[i])
				}
				m.favorites.albumCount = len(m.favorites.albums)
			case favtui.CategoryTracks:
				for i, idx := range a.indices {
					m.favorites.tracks = insertTrackAt(m.favorites.tracks, min(idx, len(m.favorites.tracks)), a.tracks[i])
				}
				m.favorites.trackCount = len(m.favorites.tracks)
			}
			if m.favorites.category == a.category {
				m.favorites = m.favorites.clampCursor()
			}
		case favtui.TrackLevel:
			for i, idx := range a.indices {
				if i >= len(a.tracks) {
					continue
				}
				hash := a.tracks[i].Hash
				if hash != "" && indexOfTrackByHash(m.favorites.drilldownTracks, hash) >= 0 {
					continue
				}
				m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, min(idx, len(m.favorites.drilldownTracks)), a.tracks[i])
			}
			m.favorites.selectedItems = nil
			m.favorites = m.favorites.clampCursor()
		}
	}
	if m.activeView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = ac
			m.favorites.albumCount = al
			m.favorites.trackCount = t
		}
	}
	n := len(a.values)
	m.statusBar.Notify(fmt.Sprintf("Undone: added %d %s(s) back to favorites", n, a.typ))
	return m, nil
}
