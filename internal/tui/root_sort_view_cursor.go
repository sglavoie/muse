package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/layout"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func (m Model) preservePlaylistCursorOnDirChange(dir sorting.SortDir) Model {
	if m.playlists.level == pltui.ListLevel {
		var focusedID int64
		pl := m.playlists.effectivePlaylists()
		if m.playlists.cursor >= 0 && m.playlists.cursor < len(pl) {
			focusedID = pl[m.playlists.cursor].ID
		}
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = sorting.FindPlaylistPosition(m.playlists.effectivePlaylists(), focusedID)
	} else {
		prevTrackIdx := m.playlists.trackIndex(m.playlists.cursor)
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = m.playlists.findTrackDisplayPosition(prevTrackIdx)
	}
	plReserve := playlistsVisibleReserve(m.playlists)
	m.playlists.scrollOffset = sorting.AdjustScroll(m.playlists.cursor, m.playlists.scrollOffset, layout.VisibleRows(m.playlists.height, plReserve))
	return m
}

func (m Model) preserveArtistsCursorOnDirChange(dir sorting.SortDir) Model {
	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.browse.artists.artists, m.browse.artists.filter.Text)
		if m.browse.artists.cursor >= 0 && m.browse.artists.cursor < len(items) {
			name = items[m.browse.artists.cursor]
		}
		m.browse.artists.sortState.Dir = dir
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.browse.artists.artists, m.browse.artists.filter.Text)
		m.browse.artists.cursor = sorting.FindStringPosition(items, name)
	case arttui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.browse.artists.albums, m.browse.artists.filter.Text)
		if m.browse.artists.cursor >= 0 && m.browse.artists.cursor < len(items) {
			name = items[m.browse.artists.cursor]
		}
		m.browse.artists.sortState.Dir = dir
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.browse.artists.albums, m.browse.artists.filter.Text)
		m.browse.artists.cursor = sorting.FindStringPosition(items, name)
	case arttui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.artists.tracks, m.browse.artists.filter.Text)
		if m.browse.artists.cursor >= 0 && m.browse.artists.cursor < len(items) {
			focusedHash = items[m.browse.artists.cursor].Hash
		}
		m.browse.artists.sortState.Dir = dir
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		items = arttui.FilterTracks(m.browse.artists.tracks, m.browse.artists.filter.Text)
		m.browse.artists.cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := artistsVisibleReserve(m.browse.artists)
	m.browse.artists.scrollOffset = sorting.AdjustScroll(m.browse.artists.cursor, m.browse.artists.scrollOffset, layout.VisibleRows(m.browse.artists.height, reserve))
	return m
}

func (m Model) preserveAlbumsCursorOnDirChange(dir sorting.SortDir) Model {
	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.browse.albums.albums, m.browse.albums.filter.Text)
		if m.browse.albums.cursor >= 0 && m.browse.albums.cursor < len(items) {
			name = items[m.browse.albums.cursor]
		}
		m.browse.albums.sortState.Dir = dir
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		items = arttui.FilterStrings(m.browse.albums.albums, m.browse.albums.filter.Text)
		m.browse.albums.cursor = sorting.FindStringPosition(items, name)
	case albtui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.albums.tracks, m.browse.albums.filter.Text)
		if m.browse.albums.cursor >= 0 && m.browse.albums.cursor < len(items) {
			focusedHash = items[m.browse.albums.cursor].Hash
		}
		m.browse.albums.sortState.Dir = dir
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		items = arttui.FilterTracks(m.browse.albums.tracks, m.browse.albums.filter.Text)
		m.browse.albums.cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := albumsVisibleReserve(m.browse.albums)
	m.browse.albums.scrollOffset = sorting.AdjustScroll(m.browse.albums.cursor, m.browse.albums.scrollOffset, layout.VisibleRows(m.browse.albums.height, reserve))
	return m
}

func (m Model) preserveFavoritesCursorOnDirChange(dir sorting.SortDir) Model {
	switch {
	case m.favorites.level == favtui.ListLevel && (m.favorites.category == favtui.CategoryArtists || m.favorites.category == favtui.CategoryAlbums):
		var name string
		var items []string
		if m.favorites.category == favtui.CategoryArtists {
			items = arttui.FilterStrings(m.favorites.artists, m.favorites.filter.Text)
		} else {
			items = arttui.FilterStrings(m.favorites.albums, m.favorites.filter.Text)
		}
		if m.favorites.cursor >= 0 && m.favorites.cursor < len(items) {
			name = items[m.favorites.cursor]
		}
		m.favorites.sortState.Dir = dir
		m.favorites = m.favorites.recomputeFavoritesSort()
		if m.favorites.category == favtui.CategoryArtists {
			items = arttui.FilterStrings(m.favorites.artists, m.favorites.filter.Text)
		} else {
			items = arttui.FilterStrings(m.favorites.albums, m.favorites.filter.Text)
		}
		m.favorites.cursor = sorting.FindStringPosition(items, name)
	default:
		var tracks []library.Track
		if m.favorites.level == favtui.TrackLevel {
			tracks = arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
		} else {
			tracks = arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
		}
		var focusedHash string
		if m.favorites.cursor >= 0 && m.favorites.cursor < len(tracks) {
			focusedHash = tracks[m.favorites.cursor].Hash
		}
		m.favorites.sortState.Dir = dir
		m.favorites = m.favorites.recomputeFavoritesSort()
		if m.favorites.level == favtui.TrackLevel {
			tracks = arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
		} else {
			tracks = arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
		}
		m.favorites.cursor = sorting.FindTrackHashPosition(tracks, focusedHash)
	}
	m.favorites.selectedItems = nil
	reserve := favoritesVisibleReserve(m.favorites)
	m.favorites.scrollOffset = sorting.AdjustScroll(m.favorites.cursor, m.favorites.scrollOffset, layout.VisibleRows(m.favorites.height, reserve))
	return m
}

func (m Model) preserveGenresCursorOnDirChange(dir sorting.SortDir) Model {
	switch m.browse.genres.level {
	case gentui.GenreLevel:
		var name string
		items := arttui.FilterStrings(m.browse.genres.genres, m.browse.genres.filter.Text)
		if m.browse.genres.cursor >= 0 && m.browse.genres.cursor < len(items) {
			name = items[m.browse.genres.cursor]
		}
		m.browse.genres.sortState.Dir = dir
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		items = arttui.FilterStrings(m.browse.genres.genres, m.browse.genres.filter.Text)
		m.browse.genres.cursor = sorting.FindStringPosition(items, name)
	case gentui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.browse.genres.artists, m.browse.genres.filter.Text)
		if m.browse.genres.cursor >= 0 && m.browse.genres.cursor < len(items) {
			name = items[m.browse.genres.cursor]
		}
		m.browse.genres.sortState.Dir = dir
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		items = arttui.FilterStrings(m.browse.genres.artists, m.browse.genres.filter.Text)
		m.browse.genres.cursor = sorting.FindStringPosition(items, name)
	case gentui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.genres.tracks, m.browse.genres.filter.Text)
		if m.browse.genres.cursor >= 0 && m.browse.genres.cursor < len(items) {
			focusedHash = items[m.browse.genres.cursor].Hash
		}
		m.browse.genres.sortState.Dir = dir
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		items = arttui.FilterTracks(m.browse.genres.tracks, m.browse.genres.filter.Text)
		m.browse.genres.cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := genresVisibleReserve(m.browse.genres)
	m.browse.genres.scrollOffset = sorting.AdjustScroll(m.browse.genres.cursor, m.browse.genres.scrollOffset, layout.VisibleRows(m.browse.genres.height, reserve))
	return m
}

func (m Model) preserveTagsCursorOnDirChange(dir sorting.SortDir) Model {
	if m.tagsView.level != tagtui.TrackLevel {
		return m
	}
	items := m.tagsView.displayTracks()
	var focusedHash string
	if m.tagsView.cursor >= 0 && m.tagsView.cursor < len(items) {
		focusedHash = items[m.tagsView.cursor].Hash
	}
	m.tagsView.sortState.Dir = dir
	m.tagsView = m.tagsView.recomputeTagsSort()
	items = m.tagsView.displayTracks()
	m.tagsView.cursor = findPlaylistTrackHashPosition(items, focusedHash)
	reserve := tagsVisibleReserve(m.tagsView)
	m.tagsView.scrollOffset = sorting.AdjustScroll(m.tagsView.cursor, m.tagsView.scrollOffset, layout.VisibleRows(m.tagsView.height, reserve))
	return m
}

func findPlaylistTrackHashPosition(tracks []db.PlaylistTrackRow, hash string) int {
	if hash == "" {
		return 0
	}
	for i, t := range tracks {
		if t.Hash == hash {
			return i
		}
	}
	return 0
}
