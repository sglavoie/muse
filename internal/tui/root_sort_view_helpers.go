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
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// artistsSortFieldsForLevel returns the sort field cycle for the given artists level.
func artistsSortFieldsForLevel(level arttui.Level) []SortField {
	switch level {
	case arttui.ArtistLevel:
		return artistArtistSortFields
	case arttui.AlbumLevel:
		return artistAlbumSortFields
	case arttui.TrackLevel:
		return artistTrackSortFields
	default:
		return artistArtistSortFields
	}
}

// albumsSortFieldsForLevel returns the sort field cycle for the given albums level.
func albumsSortFieldsForLevel(level albtui.Level) []SortField {
	switch level {
	case albtui.AlbumLevel:
		return albumAlbumSortFields
	case albtui.TrackLevel:
		return albumTrackSortFields
	default:
		return albumAlbumSortFields
	}
}

// favoritesSortFieldsForLevel returns the sort field cycle for the current
// favorites level. Returns nil at CategoryLevel (sorting not applicable).
func favoritesSortFieldsForLevel(v FavoritesView) []SortField {
	switch v.level {
	case favtui.CategoryLevel:
		return nil
	case favtui.ListLevel:
		switch v.category {
		case favtui.CategoryArtists, favtui.CategoryAlbums:
			return favListSortFields
		case favtui.CategoryTracks:
			return favTrackSortFields
		}
		return nil
	case favtui.TrackLevel:
		return favTrackSortFields
	default:
		return nil
	}
}

// genresSortFieldsForLevel returns the sort field cycle for the given genres level.
func genresSortFieldsForLevel(level gentui.Level) []SortField {
	switch level {
	case gentui.GenreLevel:
		return genreGenreSortFields
	case gentui.ArtistLevel:
		return genreArtistSortFields
	case gentui.TrackLevel:
		return genreTrackSortFields
	default:
		return genreGenreSortFields
	}
}

// preservePlaylistCursorOnDirChange changes the playlists sort direction while
// keeping the focused item at its new position.
func (m Model) preservePlaylistCursorOnDirChange(dir SortDir) Model {
	if m.playlists.level == pltui.ListLevel {
		var focusedID int64
		pl := m.playlists.effectivePlaylists()
		if m.playlists.cursor >= 0 && m.playlists.cursor < len(pl) {
			focusedID = pl[m.playlists.cursor].ID
		}
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = findPlaylistPosition(m.playlists.effectivePlaylists(), focusedID)
	} else {
		prevTrackIdx := m.playlists.trackIndex(m.playlists.cursor)
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = m.playlists.findTrackDisplayPosition(prevTrackIdx)
	}
	plReserve := playlistsVisibleReserve(m.playlists)
	m.playlists.scrollOffset = adjustScroll(m.playlists.cursor, m.playlists.scrollOffset, layout.VisibleRows(m.playlists.height, plReserve))
	return m
}

// preserveArtistsCursorOnDirChange changes the artists sort direction while
// keeping the focused item at its new position.
func (m Model) preserveArtistsCursorOnDirChange(dir SortDir) Model {
	switch m.artists.level {
	case arttui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			name = items[m.artists.cursor]
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)
		m.artists.cursor = findStringPosition(items, name)
	case arttui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.artists.albums, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			name = items[m.artists.cursor]
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.artists.albums, m.artists.filter.Text)
		m.artists.cursor = findStringPosition(items, name)
	case arttui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.artists.tracks, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			focusedHash = items[m.artists.cursor].Hash
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterTracks(m.artists.tracks, m.artists.filter.Text)
		m.artists.cursor = findTrackHashPosition(items, focusedHash)
	}
	reserve := artistsVisibleReserve(m.artists)
	m.artists.scrollOffset = adjustScroll(m.artists.cursor, m.artists.scrollOffset, layout.VisibleRows(m.artists.height, reserve))
	return m
}

// preserveAlbumsCursorOnDirChange changes the albums sort direction while
// keeping the focused item at its new position.
func (m Model) preserveAlbumsCursorOnDirChange(dir SortDir) Model {
	switch m.albums.level {
	case albtui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)
		if m.albums.cursor >= 0 && m.albums.cursor < len(items) {
			name = items[m.albums.cursor]
		}
		m.albums.sortState.Dir = dir
		m.albums = m.albums.recomputeAlbumsSort()
		items = arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)
		m.albums.cursor = findStringPosition(items, name)
	case albtui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.albums.tracks, m.albums.filter.Text)
		if m.albums.cursor >= 0 && m.albums.cursor < len(items) {
			focusedHash = items[m.albums.cursor].Hash
		}
		m.albums.sortState.Dir = dir
		m.albums = m.albums.recomputeAlbumsSort()
		items = arttui.FilterTracks(m.albums.tracks, m.albums.filter.Text)
		m.albums.cursor = findTrackHashPosition(items, focusedHash)
	}
	reserve := albumsVisibleReserve(m.albums)
	m.albums.scrollOffset = adjustScroll(m.albums.cursor, m.albums.scrollOffset, layout.VisibleRows(m.albums.height, reserve))
	return m
}

// preserveFavoritesCursorOnDirChange changes the favorites sort direction while
// keeping the focused item at its new position.
func (m Model) preserveFavoritesCursorOnDirChange(dir SortDir) Model {
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
		m.favorites.cursor = findStringPosition(items, name)
	default:
		// ListLevel+Tracks or TrackLevel: track-based sort.
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
		m.favorites.cursor = findTrackHashPosition(tracks, focusedHash)
	}
	m.favorites.selectedItems = nil
	reserve := favoritesVisibleReserve(m.favorites)
	m.favorites.scrollOffset = adjustScroll(m.favorites.cursor, m.favorites.scrollOffset, layout.VisibleRows(m.favorites.height, reserve))
	return m
}

// preserveGenresCursorOnDirChange changes the genres sort direction while
// keeping the focused item at its new position.
func (m Model) preserveGenresCursorOnDirChange(dir SortDir) Model {
	switch m.genres.level {
	case gentui.GenreLevel:
		var name string
		items := arttui.FilterStrings(m.genres.genres, m.genres.filter.Text)
		if m.genres.cursor >= 0 && m.genres.cursor < len(items) {
			name = items[m.genres.cursor]
		}
		m.genres.sortState.Dir = dir
		m.genres = m.genres.recomputeGenresSort()
		items = arttui.FilterStrings(m.genres.genres, m.genres.filter.Text)
		m.genres.cursor = findStringPosition(items, name)
	case gentui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.genres.artists, m.genres.filter.Text)
		if m.genres.cursor >= 0 && m.genres.cursor < len(items) {
			name = items[m.genres.cursor]
		}
		m.genres.sortState.Dir = dir
		m.genres = m.genres.recomputeGenresSort()
		items = arttui.FilterStrings(m.genres.artists, m.genres.filter.Text)
		m.genres.cursor = findStringPosition(items, name)
	case gentui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.genres.tracks, m.genres.filter.Text)
		if m.genres.cursor >= 0 && m.genres.cursor < len(items) {
			focusedHash = items[m.genres.cursor].Hash
		}
		m.genres.sortState.Dir = dir
		m.genres = m.genres.recomputeGenresSort()
		items = arttui.FilterTracks(m.genres.tracks, m.genres.filter.Text)
		m.genres.cursor = findTrackHashPosition(items, focusedHash)
	}
	reserve := genresVisibleReserve(m.genres)
	m.genres.scrollOffset = adjustScroll(m.genres.cursor, m.genres.scrollOffset, layout.VisibleRows(m.genres.height, reserve))
	return m
}

// libraryViewVisibleReserve returns the line reserve for a LibraryView's visibleRows.
// The reserve must match library.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func libraryViewVisibleReserve(v LibraryView) int {
	reserve := 1 // column header
	if v.filter.Active || v.filter.Text != "" {
		reserve++
	}
	return reserve
}

// playlistsVisibleReserve returns the line reserve for the playlists view's visibleRows.
// The reserve must match playlists.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func playlistsVisibleReserve(v PlaylistsView) int {
	reserve := 1
	if v.level == pltui.ListLevel && v.sortState.Field != SortDefault {
		reserve++
	}
	if v.level == pltui.TrackLevel {
		reserve = 4
	}
	return reserve
}

// artistsVisibleReserve returns the line reserve for the artists view's visibleRows.
// The reserve must match artists.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func artistsVisibleReserve(v ArtistsView) int {
	var reserve int
	switch v.level {
	case arttui.ArtistLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default:
		reserve = 3
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// albumsVisibleReserve returns the line reserve for the albums view's visibleRows.
// The reserve must match albums.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func albumsVisibleReserve(v AlbumsView) int {
	var reserve int
	switch v.level {
	case albtui.AlbumLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default:
		reserve = 3
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// favoritesVisibleReserve returns the line reserve for the favorites view's visibleRows.
// The reserve must match favorites.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func favoritesVisibleReserve(v FavoritesView) int {
	var reserve int
	switch v.level {
	case favtui.CategoryLevel:
		// no extra reserve
	case favtui.ListLevel:
		reserve = 2 // heading + separator (blank line or filter line)
		if v.category == favtui.CategoryTracks {
			reserve++ // column header row
		}
	case favtui.TrackLevel:
		reserve = 3
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// genresVisibleReserve returns the line reserve for the genres view's visibleRows.
// The reserve must match genres.View.visibleRows() so that adjustScroll and
// the cursor-movement helpers agree on how many rows are visible.
func genresVisibleReserve(v GenresView) int {
	var reserve int
	switch v.level {
	case gentui.GenreLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default:
		reserve = 3
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// tagsSortFieldsForLevel returns the sort field cycle for the given tags level.
// Returns nil at TagLevel (sorting not applicable to the tag list).
func tagsSortFieldsForLevel(level tagtui.Level) []SortField {
	switch level {
	case tagtui.TrackLevel:
		return tagsTrackSortFields
	default:
		return nil
	}
}

// preserveTagsCursorOnDirChange changes the tags sort direction while keeping
// the focused item at its new position.
func (m Model) preserveTagsCursorOnDirChange(dir SortDir) Model {
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
	m.tagsView.scrollOffset = adjustScroll(m.tagsView.cursor, m.tagsView.scrollOffset, layout.VisibleRows(m.tagsView.height, reserve))
	return m
}

// findPlaylistTrackHashPosition returns the index of the track with the given
// hash in a PlaylistTrackRow slice, or 0 if not found.
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
