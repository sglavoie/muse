package tui

import (
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// resetLibrarySort resets the library sort state to the mode's default.
// For LibraryModeAll this clears any custom sort; for Recent and History modes
// it restores their fixed sort (Date Added and Last Played respectively).
func (m Model) resetLibrarySort() Model {
	switch m.library.Mode {
	case libtui.LibraryModeRecent:
		m.library.SortState = sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}
		m.library = m.library.RecomputeSortedTracks()
		if m.library.Filter.Text != "" {
			m.library = m.library.RebuildFilterCache()
		}
	case libtui.LibraryModeHistory:
		m.library.SortState = sorting.SortState{Field: sorting.SortLastPlayed, Dir: sorting.SortDesc}
		m.library = m.library.RecomputeSortedTracks()
		if m.library.Filter.Text != "" {
			m.library = m.library.RebuildFilterCache()
		}
	default: // LibraryModeAll
		hadSort := m.library.SortState.Field != sorting.SortDefault
		m.library.SortState = sorting.SortState{}
		m.library.SortedTracks = nil
		if hadSort && m.library.Filter.Text != "" {
			m.library = m.library.RebuildFilterCache()
		}
	}
	return m
}

// resetQueueSort clears the queue sort state and display order.
func (m Model) resetQueueSort() Model {
	m.queue.SortState = sorting.SortState{}
	m.queue.DisplayOrder = nil
	return m
}

// preserveQueueSort recomputes displayOrder if a sort is active, otherwise
// leaves the queue in insertion-order mode.
func (m Model) preserveQueueSort() Model {
	if m.queue.SortState.Field != sorting.SortDefault {
		m.queue = m.queue.RecomputeDisplayOrder()
	}
	return m
}

// resetPlaylistsSort clears the playlists sort state and sorted caches.
func (m Model) resetPlaylistsSort() Model {
	m.playlists.SortState = sorting.SortState{}
	m.playlists.SortedPlaylists = nil
	m.playlists.SortedTrackOrder = nil
	return m
}

// preservePlaylistsSort recomputes playlist sort caches if a sort is active,
// otherwise leaves the view in default order.
func (m Model) preservePlaylistsSort() Model {
	if m.playlists.SortState.Field != sorting.SortDefault {
		m.playlists = m.playlists.RecomputePlaylistSort()
	}
	return m
}

// resetArtistsSort clears the artists sort state. When a non-default sort was
// active the backing slices were sorted in-place, so re-derive from the library
// index to restore the original order.
func (m Model) resetArtistsSort() Model {
	hadSort := m.browse.Artists.SortState.Field != sorting.SortDefault
	m.browse.Artists.SortState = sorting.SortState{}
	if hadSort {
		m = m.rederiveArtistsData()
	}
	return m
}

// resetAlbumsSort clears the albums sort state. See resetArtistsSort.
func (m Model) resetAlbumsSort() Model {
	hadSort := m.browse.Albums.SortState.Field != sorting.SortDefault
	m.browse.Albums.SortState = sorting.SortState{}
	if hadSort {
		m = m.rederiveAlbumsData()
	}
	return m
}

// resetFavoritesSort clears the favorites sort state. See resetArtistsSort.
func (m Model) resetFavoritesSort() Model {
	hadSort := m.favorites.SortState.Field != sorting.SortDefault
	m.favorites.SortState = sorting.SortState{}
	if hadSort {
		m = m.rederiveFavoritesData()
	}
	return m
}

// resetGenresSort clears the genres sort state. See resetArtistsSort.
func (m Model) resetGenresSort() Model {
	hadSort := m.browse.Genres.SortState.Field != sorting.SortDefault
	m.browse.Genres.SortState = sorting.SortState{}
	if hadSort {
		m = m.rederiveGenresData()
	}
	return m
}

// resetTagsSort clears the tags sort state.
func (m Model) resetTagsSort() Model {
	m.tagsView.SortState = sorting.SortState{}
	m.tagsView.SortedTrackOrder = nil
	return m
}

// resetAllSorts clears sort state for all views.
func (m Model) resetAllSorts() Model {
	m = m.resetLibrarySort()
	m = m.resetQueueSort()
	m = m.resetPlaylistsSort()
	m = m.resetArtistsSort()
	m = m.resetAlbumsSort()
	m = m.resetFavoritesSort()
	m = m.resetGenresSort()
	m = m.resetTagsSort()
	return m
}
