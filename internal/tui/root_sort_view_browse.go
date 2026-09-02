package tui

import "github.com/sglavoie/muse/internal/tui/sorting"

func (m Model) shiftBrowseSortDir(dir sorting.SortDir) (Model, sorting.SortState, bool) {
	switch m.browse.mode {
	case BrowseModeArtists:
		if m.browse.artists.sortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveArtistsCursorOnDirChange(dir)
		return m, m.browse.artists.sortState, true
	case BrowseModeAlbums:
		if m.browse.albums.sortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveAlbumsCursorOnDirChange(dir)
		return m, m.browse.albums.sortState, true
	case BrowseModeGenres:
		if m.browse.genres.sortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveGenresCursorOnDirChange(dir)
		return m, m.browse.genres.sortState, true
	}
	return m, sorting.SortState{}, false
}

func (m Model) cycleBrowseSortField() (Model, sorting.SortState, bool) {
	switch m.browse.mode {
	case BrowseModeArtists:
		fields := artistsSortFieldsForLevel(m.browse.artists.level)
		next := sorting.CycleSortField(m.browse.artists.sortState.Field, fields)
		m.browse.artists.sortState.Field = next
		if next == sorting.SortDefault {
			m.browse.artists.sortState.Dir = sorting.SortAsc
			m = m.rederiveArtistsData()
		}
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists.cursor = 0
		m.browse.artists.scrollOffset = 0
		return m, m.browse.artists.sortState, true
	case BrowseModeAlbums:
		fields := albumsSortFieldsForLevel(m.browse.albums.level)
		next := sorting.CycleSortField(m.browse.albums.sortState.Field, fields)
		m.browse.albums.sortState.Field = next
		if next == sorting.SortDefault {
			m.browse.albums.sortState.Dir = sorting.SortAsc
			m = m.rederiveAlbumsData()
		}
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		m.browse.albums.cursor = 0
		m.browse.albums.scrollOffset = 0
		return m, m.browse.albums.sortState, true
	case BrowseModeGenres:
		fields := genresSortFieldsForLevel(m.browse.genres.level)
		next := sorting.CycleSortField(m.browse.genres.sortState.Field, fields)
		m.browse.genres.sortState.Field = next
		if next == sorting.SortDefault {
			m.browse.genres.sortState.Dir = sorting.SortAsc
			m = m.rederiveGenresData()
		}
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres.cursor = 0
		m.browse.genres.scrollOffset = 0
		return m, m.browse.genres.sortState, true
	}
	return m, sorting.SortState{}, false
}
