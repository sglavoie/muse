package tui

// resetLibrarySort resets the library sort state to the mode's default.
// For LibraryModeAll this clears any custom sort; for Recent and History modes
// it restores their fixed sort (Date Added and Last Played respectively).
func (m Model) resetLibrarySort() Model {
	switch m.library.mode {
	case LibraryModeRecent:
		m.library.sortState = SortState{Field: SortDateAdded, Dir: SortDesc}
		m.library = m.library.recomputeSortedTracks()
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
	case LibraryModeHistory:
		m.library.sortState = SortState{Field: SortLastPlayed, Dir: SortDesc}
		m.library = m.library.recomputeSortedTracks()
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
	default: // LibraryModeAll
		hadSort := m.library.sortState.Field != SortDefault
		m.library.sortState = SortState{}
		m.library.sortedTracks = nil
		if hadSort && m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
	}
	return m
}

// resetQueueSort clears the queue sort state and display order.
func (m Model) resetQueueSort() Model {
	m.queue.sortState = SortState{}
	m.queue.displayOrder = nil
	return m
}

// preserveQueueSort recomputes displayOrder if a sort is active, otherwise
// leaves the queue in insertion-order mode.
func (m Model) preserveQueueSort() Model {
	if m.queue.sortState.Field != SortDefault {
		m.queue = m.queue.recomputeDisplayOrder()
	}
	return m
}

// resetPlaylistsSort clears the playlists sort state and sorted caches.
func (m Model) resetPlaylistsSort() Model {
	m.playlists.sortState = SortState{}
	m.playlists.sortedPlaylists = nil
	m.playlists.sortedTrackOrder = nil
	return m
}

// preservePlaylistsSort recomputes playlist sort caches if a sort is active,
// otherwise leaves the view in default order.
func (m Model) preservePlaylistsSort() Model {
	if m.playlists.sortState.Field != SortDefault {
		m.playlists = m.playlists.recomputePlaylistSort()
	}
	return m
}

// resetArtistsSort clears the artists sort state. When a non-default sort was
// active the backing slices were sorted in-place, so re-derive from the library
// index to restore the original order.
func (m Model) resetArtistsSort() Model {
	hadSort := m.browse.artists.sortState.Field != SortDefault
	m.browse.artists.sortState = SortState{}
	if hadSort {
		m = m.rederiveArtistsData()
	}
	return m
}

// resetAlbumsSort clears the albums sort state. See resetArtistsSort.
func (m Model) resetAlbumsSort() Model {
	hadSort := m.browse.albums.sortState.Field != SortDefault
	m.browse.albums.sortState = SortState{}
	if hadSort {
		m = m.rederiveAlbumsData()
	}
	return m
}

// resetFavoritesSort clears the favorites sort state. See resetArtistsSort.
func (m Model) resetFavoritesSort() Model {
	hadSort := m.favorites.sortState.Field != SortDefault
	m.favorites.sortState = SortState{}
	if hadSort {
		m = m.rederiveFavoritesData()
	}
	return m
}

// resetGenresSort clears the genres sort state. See resetArtistsSort.
func (m Model) resetGenresSort() Model {
	hadSort := m.browse.genres.sortState.Field != SortDefault
	m.browse.genres.sortState = SortState{}
	if hadSort {
		m = m.rederiveGenresData()
	}
	return m
}

// resetTagsSort clears the tags sort state.
func (m Model) resetTagsSort() Model {
	m.tagsView.sortState = SortState{}
	m.tagsView.sortedTrackOrder = nil
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
