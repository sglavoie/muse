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

// resetTagsSort clears the tags sort state.
func (m Model) resetTagsSort() Model {
	m.tagsView.sortState = SortState{}
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
