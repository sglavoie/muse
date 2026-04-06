package tui

// resetLibrarySort clears the library sort state and cached sorted tracks.
// When a filter is active, the cached fuzzy-match results are rebuilt against
// the unsorted track list so that TrackIndex values remain valid.
func (m Model) resetLibrarySort() Model {
	hadSort := m.library.sortState.Field != SortDefault
	m.library.sortState = SortState{}
	m.library.sortedTracks = nil
	if hadSort && m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	return m
}

// resetRecentlyAddedSort restores the fixed Date Added sort state.
func (m Model) resetRecentlyAddedSort() Model {
	m.recentlyAdded.sortState = SortState{Field: SortDateAdded, Dir: SortDesc}
	m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()
	if m.recentlyAdded.filter.Text != "" {
		m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
	}
	return m
}

// resetQueueSort clears the queue sort state and display order.
func (m Model) resetQueueSort() Model {
	m.queue.sortState = SortState{}
	m.queue.displayOrder = nil
	return m
}

// resetPlaylistsSort clears the playlists sort state and sorted caches.
func (m Model) resetPlaylistsSort() Model {
	m.playlists.sortState = SortState{}
	m.playlists.sortedPlaylists = nil
	m.playlists.sortedTrackOrder = nil
	return m
}

// resetArtistsSort clears the artists sort state.
func (m Model) resetArtistsSort() Model {
	m.artists.sortState = SortState{}
	return m
}

// resetAlbumsSort clears the albums sort state.
func (m Model) resetAlbumsSort() Model {
	m.albums.sortState = SortState{}
	return m
}

// resetFavoritesSort clears the favorites sort state.
func (m Model) resetFavoritesSort() Model {
	m.favorites.sortState = SortState{}
	return m
}

// resetGenresSort clears the genres sort state.
func (m Model) resetGenresSort() Model {
	m.genres.sortState = SortState{}
	return m
}

// resetAllSorts clears sort state for all views.
func (m Model) resetAllSorts() Model {
	m = m.resetLibrarySort()
	m = m.resetRecentlyAddedSort()
	m = m.resetQueueSort()
	m = m.resetPlaylistsSort()
	m = m.resetArtistsSort()
	m = m.resetAlbumsSort()
	m = m.resetFavoritesSort()
	m = m.resetGenresSort()
	return m
}
