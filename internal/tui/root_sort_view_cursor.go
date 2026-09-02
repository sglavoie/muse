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
	if m.playlists.Level == pltui.ListLevel {
		var focusedID int64
		pl := m.playlists.EffectivePlaylists()
		if m.playlists.Cursor >= 0 && m.playlists.Cursor < len(pl) {
			focusedID = pl[m.playlists.Cursor].ID
		}
		m.playlists.SortState.Dir = dir
		m.playlists = m.playlists.RecomputePlaylistSort()
		m.playlists.Cursor = sorting.FindPlaylistPosition(m.playlists.EffectivePlaylists(), focusedID)
	} else {
		prevTrackIdx := m.playlists.TrackIndex(m.playlists.Cursor)
		m.playlists.SortState.Dir = dir
		m.playlists = m.playlists.RecomputePlaylistSort()
		m.playlists.Cursor = m.playlists.FindTrackDisplayPosition(prevTrackIdx)
	}
	plReserve := playlistsVisibleReserve(m.playlists)
	m.playlists.ScrollOffset = sorting.AdjustScroll(m.playlists.Cursor, m.playlists.ScrollOffset, layout.VisibleRows(m.playlists.Height, plReserve))
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
	case m.favorites.Level == favtui.ListLevel && (m.favorites.Category == favtui.CategoryArtists || m.favorites.Category == favtui.CategoryAlbums):
		var name string
		var items []string
		if m.favorites.Category == favtui.CategoryArtists {
			items = arttui.FilterStrings(m.favorites.Artists, m.favorites.Filter.Text)
		} else {
			items = arttui.FilterStrings(m.favorites.Albums, m.favorites.Filter.Text)
		}
		if m.favorites.Cursor >= 0 && m.favorites.Cursor < len(items) {
			name = items[m.favorites.Cursor]
		}
		m.favorites.SortState.Dir = dir
		m.favorites = m.favorites.RecomputeFavoritesSort()
		if m.favorites.Category == favtui.CategoryArtists {
			items = arttui.FilterStrings(m.favorites.Artists, m.favorites.Filter.Text)
		} else {
			items = arttui.FilterStrings(m.favorites.Albums, m.favorites.Filter.Text)
		}
		m.favorites.Cursor = sorting.FindStringPosition(items, name)
	default:
		var tracks []library.Track
		if m.favorites.Level == favtui.TrackLevel {
			tracks = arttui.FilterTracks(m.favorites.DrilldownTracks, m.favorites.Filter.Text)
		} else {
			tracks = arttui.FilterTracks(m.favorites.Tracks, m.favorites.Filter.Text)
		}
		var focusedHash string
		if m.favorites.Cursor >= 0 && m.favorites.Cursor < len(tracks) {
			focusedHash = tracks[m.favorites.Cursor].Hash
		}
		m.favorites.SortState.Dir = dir
		m.favorites = m.favorites.RecomputeFavoritesSort()
		if m.favorites.Level == favtui.TrackLevel {
			tracks = arttui.FilterTracks(m.favorites.DrilldownTracks, m.favorites.Filter.Text)
		} else {
			tracks = arttui.FilterTracks(m.favorites.Tracks, m.favorites.Filter.Text)
		}
		m.favorites.Cursor = sorting.FindTrackHashPosition(tracks, focusedHash)
	}
	m.favorites.SelectedItems = nil
	reserve := favoritesVisibleReserve(m.favorites)
	m.favorites.ScrollOffset = sorting.AdjustScroll(m.favorites.Cursor, m.favorites.ScrollOffset, layout.VisibleRows(m.favorites.Height, reserve))
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
	if m.tagsView.Level != tagtui.TrackLevel {
		return m
	}
	items := m.tagsView.DisplayTracks()
	var focusedHash string
	if m.tagsView.Cursor >= 0 && m.tagsView.Cursor < len(items) {
		focusedHash = items[m.tagsView.Cursor].Hash
	}
	m.tagsView.SortState.Dir = dir
	m.tagsView = m.tagsView.RecomputeTagsSort()
	items = m.tagsView.DisplayTracks()
	m.tagsView.Cursor = findPlaylistTrackHashPosition(items, focusedHash)
	reserve := tagsVisibleReserve(m.tagsView)
	m.tagsView.ScrollOffset = sorting.AdjustScroll(m.tagsView.Cursor, m.tagsView.ScrollOffset, layout.VisibleRows(m.tagsView.Height, reserve))
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
