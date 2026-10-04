package tui

import (
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func (m Model) shiftBrowseSortDir(dir sorting.SortDir) (Model, sorting.SortState, bool) {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if m.browse.Artists.SortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveArtistsCursorOnDirChange(dir)
		return m, m.browse.Artists.SortState, true
	case browse.BrowseModeAlbums:
		if m.browse.Albums.SortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveAlbumsCursorOnDirChange(dir)
		return m, m.browse.Albums.SortState, true
	case browse.BrowseModeGenres:
		if m.browse.Genres.SortState.Field == sorting.SortDefault {
			return m, sorting.SortState{}, false
		}
		m = m.preserveGenresCursorOnDirChange(dir)
		return m, m.browse.Genres.SortState, true
	}
	return m, sorting.SortState{}, false
}

func (m Model) applyBrowseSortField(next sorting.SortField) (Model, sorting.SortState, bool) {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		m.browse.Artists.SortState.Field = next
		if next == sorting.SortDefault {
			m.browse.Artists.SortState.Dir = sorting.SortAsc
			m = m.rederiveArtistsData()
		}
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
		m.browse.Artists.Cursor = 0
		m.browse.Artists.ScrollOffset = 0
		return m, m.browse.Artists.SortState, true
	case browse.BrowseModeAlbums:
		m.browse.Albums.SortState.Field = next
		if next == sorting.SortDefault {
			m.browse.Albums.SortState.Dir = sorting.SortAsc
			m = m.rederiveAlbumsData()
		}
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
		m.browse.Albums.Cursor = 0
		m.browse.Albums.ScrollOffset = 0
		return m, m.browse.Albums.SortState, true
	case browse.BrowseModeGenres:
		m.browse.Genres.SortState.Field = next
		if next == sorting.SortDefault {
			m.browse.Genres.SortState.Dir = sorting.SortAsc
			m = m.rederiveGenresData()
		}
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
		m.browse.Genres.Cursor = 0
		m.browse.Genres.ScrollOffset = 0
		return m, m.browse.Genres.SortState, true
	}
	return m, sorting.SortState{}, false
}
