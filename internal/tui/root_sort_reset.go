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
	hadSort := m.artists.sortState.Field != SortDefault
	m.artists.sortState = SortState{}
	if hadSort {
		m = m.rederiveArtistsData()
	}
	return m
}

// resetAlbumsSort clears the albums sort state. See resetArtistsSort.
func (m Model) resetAlbumsSort() Model {
	hadSort := m.albums.sortState.Field != SortDefault
	m.albums.sortState = SortState{}
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
	hadSort := m.genres.sortState.Field != SortDefault
	m.genres.sortState = SortState{}
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

// resetHistorySort restores the fixed Last Played sort state.
func (m Model) resetHistorySort() Model {
	m.history.sortState = SortState{Field: SortLastPlayed, Dir: SortDesc}
	m.history = m.history.recomputeSortedTracks()
	if m.history.filter.Text != "" {
		m.history = m.history.rebuildFilterCache()
	}
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
	m = m.resetTagsSort()
	m = m.resetHistorySort()
	return m
}
