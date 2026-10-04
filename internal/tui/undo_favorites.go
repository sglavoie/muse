package tui

import (
	"fmt"
	"log/slog"

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

func (a undoRemoveFavoriteAction) perform(m Model) (Model, error) {
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
		m.djInvalidate()
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
	if m.ActiveView == viewFavorites && a.idx >= 0 &&
		m.favorites.Level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				m.favorites.Artists = insertStringAt(m.favorites.Artists, a.idx, a.value)
				m.favorites.ArtistCount = len(m.favorites.Artists)
			case favtui.CategoryAlbums:
				m.favorites.Albums = insertStringAt(m.favorites.Albums, a.idx, a.value)
				m.favorites.AlbumCount = len(m.favorites.Albums)
			case favtui.CategoryTracks:
				m.favorites.Tracks = insertTrackAt(m.favorites.Tracks, a.idx, a.track)
				m.favorites.TrackCount = len(m.favorites.Tracks)
			}
			if m.favorites.Category == a.category {
				m.favorites.Cursor = a.idx
				m.favorites = m.favorites.ClampCursor()
			}
		case favtui.TrackLevel:
			hash := a.value
			if hash == "" {
				hash = a.track.Hash
			}
			if hash != "" && indexOfTrackByHash(m.favorites.DrilldownTracks, hash) < 0 && a.track.Hash != "" {
				m.favorites.DrilldownTracks = insertTrackAt(m.favorites.DrilldownTracks, a.idx, a.track)
			}
			m.favorites.SelectedItems = nil
			if a.idx >= 0 {
				m.favorites.Cursor = a.idx
			}
			m.favorites = m.favorites.ClampCursor()
		}
	}
	if m.ActiveView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.ArtistCount = ac
			m.favorites.AlbumCount = al
			m.favorites.TrackCount = t
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

func (a undoAddFavoriteAction) perform(m Model) (Model, error) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavorite(a.typ, a.value); err != nil {
			slog.Warn("undo add favorite failed", "err", err)
		}
	}
	if a.typ == "track" && a.value != "" {
		delete(m.favoriteHashes, a.value)
		m.djInvalidate()
	}
	if a.typ == "artist" && a.value != "" {
		delete(m.favoriteArtists, a.value)
	}
	if a.typ == "album" && a.value != "" {
		delete(m.favoriteAlbums, a.value)
	}
	if m.ActiveView == viewFavorites {
		switch m.favorites.Level {
		case favtui.ListLevel:
			switch {
			case a.typ == "artist" && m.favorites.Category == favtui.CategoryArtists:
				m.favorites.Artists = removeString(m.favorites.Artists, a.value)
				m.favorites.ArtistCount = len(m.favorites.Artists)
			case a.typ == "album" && m.favorites.Category == favtui.CategoryAlbums:
				m.favorites.Albums = removeString(m.favorites.Albums, a.value)
				m.favorites.AlbumCount = len(m.favorites.Albums)
			case a.typ == "track" && m.favorites.Category == favtui.CategoryTracks:
				m.favorites.Tracks = removeTrackByHash(m.favorites.Tracks, a.value)
				m.favorites.TrackCount = len(m.favorites.Tracks)
			}
			m.favorites.SelectedItems = nil
			m.favorites = m.favorites.ClampCursor()
		case favtui.TrackLevel:
			if a.typ == "track" {
				m.favorites.SelectedItems = nil
				m.favorites = m.favorites.ClampCursor()
			}
		}
		if m.favoriteReader != nil {
			ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.ArtistCount = ac
				m.favorites.AlbumCount = al
				m.favorites.TrackCount = t
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

func (a undoBulkAddFavoritesAction) perform(m Model) (Model, error) {
	if m.favoriteWriter != nil {
		if err := m.favoriteWriter.RemoveFavoritesByTypeAndValues(a.typ, a.values); err != nil {
			slog.Warn("undo bulk add favorites failed", "err", err)
		}
	}
	if a.typ == "track" {
		for _, v := range a.values {
			delete(m.favoriteHashes, v)
		}
		m.djInvalidate()
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
	if m.ActiveView == viewFavorites {
		switch m.favorites.Level {
		case favtui.ListLevel:
			switch {
			case a.typ == "artist" && m.favorites.Category == favtui.CategoryArtists:
				for _, v := range a.values {
					m.favorites.Artists = removeString(m.favorites.Artists, v)
				}
				m.favorites.ArtistCount = len(m.favorites.Artists)
			case a.typ == "album" && m.favorites.Category == favtui.CategoryAlbums:
				for _, v := range a.values {
					m.favorites.Albums = removeString(m.favorites.Albums, v)
				}
				m.favorites.AlbumCount = len(m.favorites.Albums)
			case a.typ == "track" && m.favorites.Category == favtui.CategoryTracks:
				for _, v := range a.values {
					m.favorites.Tracks = removeTrackByHash(m.favorites.Tracks, v)
				}
				m.favorites.TrackCount = len(m.favorites.Tracks)
			}
			m.favorites.SelectedItems = nil
			m.favorites = m.favorites.ClampCursor()
		case favtui.TrackLevel:
			if a.typ == "track" {
				m.favorites.SelectedItems = nil
				m.favorites = m.favorites.ClampCursor()
			}
		}
		if m.favoriteReader != nil {
			ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.ArtistCount = ac
				m.favorites.AlbumCount = al
				m.favorites.TrackCount = t
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

func (a undoBulkRemoveFavoritesAction) perform(m Model) (Model, error) {
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
		m.djInvalidate()
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
	if m.ActiveView == viewFavorites && len(a.indices) > 0 &&
		m.favorites.Level == a.level {
		switch a.level {
		case favtui.ListLevel:
			switch a.category {
			case favtui.CategoryArtists:
				for i, idx := range a.indices {
					m.favorites.Artists = insertStringAt(m.favorites.Artists, min(idx, len(m.favorites.Artists)), a.values[i])
				}
				m.favorites.ArtistCount = len(m.favorites.Artists)
			case favtui.CategoryAlbums:
				for i, idx := range a.indices {
					m.favorites.Albums = insertStringAt(m.favorites.Albums, min(idx, len(m.favorites.Albums)), a.values[i])
				}
				m.favorites.AlbumCount = len(m.favorites.Albums)
			case favtui.CategoryTracks:
				for i, idx := range a.indices {
					m.favorites.Tracks = insertTrackAt(m.favorites.Tracks, min(idx, len(m.favorites.Tracks)), a.tracks[i])
				}
				m.favorites.TrackCount = len(m.favorites.Tracks)
			}
			if m.favorites.Category == a.category {
				m.favorites = m.favorites.ClampCursor()
			}
		case favtui.TrackLevel:
			for i, idx := range a.indices {
				if i >= len(a.tracks) {
					continue
				}
				hash := a.tracks[i].Hash
				if hash != "" && indexOfTrackByHash(m.favorites.DrilldownTracks, hash) >= 0 {
					continue
				}
				m.favorites.DrilldownTracks = insertTrackAt(m.favorites.DrilldownTracks, min(idx, len(m.favorites.DrilldownTracks)), a.tracks[i])
			}
			m.favorites.SelectedItems = nil
			m.favorites = m.favorites.ClampCursor()
		}
	}
	if m.ActiveView == viewFavorites && m.favoriteReader != nil {
		ac, al, t, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.ArtistCount = ac
			m.favorites.AlbumCount = al
			m.favorites.TrackCount = t
		}
	}
	n := len(a.values)
	m.statusBar.Notify(fmt.Sprintf("Undone: added %d %s(s) back to favorites", n, a.typ))
	return m, nil
}
