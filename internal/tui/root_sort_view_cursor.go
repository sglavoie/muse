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
	switch m.browse.Artists.Level {
	case arttui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.browse.Artists.Artists, m.browse.Artists.Filter.Text)
		if m.browse.Artists.Cursor >= 0 && m.browse.Artists.Cursor < len(items) {
			name = items[m.browse.Artists.Cursor]
		}
		m.browse.Artists.SortState.Dir = dir
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		items = arttui.FilterStrings(m.browse.Artists.Artists, m.browse.Artists.Filter.Text)
		m.browse.Artists.Cursor = sorting.FindStringPosition(items, name)
	case arttui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.browse.Artists.Albums, m.browse.Artists.Filter.Text)
		if m.browse.Artists.Cursor >= 0 && m.browse.Artists.Cursor < len(items) {
			name = items[m.browse.Artists.Cursor]
		}
		m.browse.Artists.SortState.Dir = dir
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		items = arttui.FilterStrings(m.browse.Artists.Albums, m.browse.Artists.Filter.Text)
		m.browse.Artists.Cursor = sorting.FindStringPosition(items, name)
	case arttui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.Artists.Tracks, m.browse.Artists.Filter.Text)
		if m.browse.Artists.Cursor >= 0 && m.browse.Artists.Cursor < len(items) {
			focusedHash = items[m.browse.Artists.Cursor].Hash
		}
		m.browse.Artists.SortState.Dir = dir
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		items = arttui.FilterTracks(m.browse.Artists.Tracks, m.browse.Artists.Filter.Text)
		m.browse.Artists.Cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := artistsVisibleReserve(m.browse.Artists)
	m.browse.Artists.ScrollOffset = sorting.AdjustScroll(m.browse.Artists.Cursor, m.browse.Artists.ScrollOffset, layout.VisibleRows(m.browse.Artists.Height, reserve))
	return m
}

func (m Model) preserveAlbumsCursorOnDirChange(dir sorting.SortDir) Model {
	switch m.browse.Albums.Level {
	case albtui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.browse.Albums.Albums, m.browse.Albums.Filter.Text)
		if m.browse.Albums.Cursor >= 0 && m.browse.Albums.Cursor < len(items) {
			name = items[m.browse.Albums.Cursor]
		}
		m.browse.Albums.SortState.Dir = dir
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
		items = arttui.FilterStrings(m.browse.Albums.Albums, m.browse.Albums.Filter.Text)
		m.browse.Albums.Cursor = sorting.FindStringPosition(items, name)
	case albtui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.Albums.Tracks, m.browse.Albums.Filter.Text)
		if m.browse.Albums.Cursor >= 0 && m.browse.Albums.Cursor < len(items) {
			focusedHash = items[m.browse.Albums.Cursor].Hash
		}
		m.browse.Albums.SortState.Dir = dir
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
		items = arttui.FilterTracks(m.browse.Albums.Tracks, m.browse.Albums.Filter.Text)
		m.browse.Albums.Cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := albumsVisibleReserve(m.browse.Albums)
	m.browse.Albums.ScrollOffset = sorting.AdjustScroll(m.browse.Albums.Cursor, m.browse.Albums.ScrollOffset, layout.VisibleRows(m.browse.Albums.Height, reserve))
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
	switch m.browse.Genres.Level {
	case gentui.GenreLevel:
		var name string
		items := arttui.FilterStrings(m.browse.Genres.Genres, m.browse.Genres.Filter.Text)
		if m.browse.Genres.Cursor >= 0 && m.browse.Genres.Cursor < len(items) {
			name = items[m.browse.Genres.Cursor]
		}
		m.browse.Genres.SortState.Dir = dir
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		items = arttui.FilterStrings(m.browse.Genres.Genres, m.browse.Genres.Filter.Text)
		m.browse.Genres.Cursor = sorting.FindStringPosition(items, name)
	case gentui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.browse.Genres.Artists, m.browse.Genres.Filter.Text)
		if m.browse.Genres.Cursor >= 0 && m.browse.Genres.Cursor < len(items) {
			name = items[m.browse.Genres.Cursor]
		}
		m.browse.Genres.SortState.Dir = dir
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		items = arttui.FilterStrings(m.browse.Genres.Artists, m.browse.Genres.Filter.Text)
		m.browse.Genres.Cursor = sorting.FindStringPosition(items, name)
	case gentui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.browse.Genres.Tracks, m.browse.Genres.Filter.Text)
		if m.browse.Genres.Cursor >= 0 && m.browse.Genres.Cursor < len(items) {
			focusedHash = items[m.browse.Genres.Cursor].Hash
		}
		m.browse.Genres.SortState.Dir = dir
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		items = arttui.FilterTracks(m.browse.Genres.Tracks, m.browse.Genres.Filter.Text)
		m.browse.Genres.Cursor = sorting.FindTrackHashPosition(items, focusedHash)
	}
	reserve := genresVisibleReserve(m.browse.Genres)
	m.browse.Genres.ScrollOffset = sorting.AdjustScroll(m.browse.Genres.Cursor, m.browse.Genres.ScrollOffset, layout.VisibleRows(m.browse.Genres.Height, reserve))
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
