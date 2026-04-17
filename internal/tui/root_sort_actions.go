package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/layout"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// handleSortAction handles sort-related key actions. Returns (model, cmd, true)
// when the action was handled, or (zero, nil, false) to let the caller continue
// the handler chain.
func (m Model) handleSortAction(a action) (tea.Model, tea.Cmd, bool) {
	switch a {
	case actionSortCycle:
		m = exitVisualMode(m)
		return m.handleSortCycle()
	case actionSortAsc:
		m = exitVisualMode(m)
		return m.handleSortDir(SortAsc)
	case actionSortDesc:
		m = exitVisualMode(m)
		return m.handleSortDir(SortDesc)
	default:
		return m, nil, false
	}
}

// handleSortCycle cycles to the next sort field for the active view.
func (m Model) handleSortCycle() (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		next := CycleSortField(m.library.sortState.Field, librarySortFields)
		m.library.sortState.Field = next
		if next == SortDefault {
			m.library.sortState.Dir = SortAsc
		}
		m.library = m.library.recomputeSortedTracks()
		m.library.cursor = 0
		m.library.scrollOffset = 0
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
		m.statusBar.notification = newNotification(sortNotification(m.library.sortState))
		return m, nil, true

	case viewRecentlyAdded:
		next := CycleSortField(m.recentlyAdded.sortState.Field, recentlyAddedSortFields)
		m.recentlyAdded.sortState.Field = next
		if next == SortDefault {
			m.recentlyAdded.sortState.Dir = SortAsc
		}
		m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()
		m.recentlyAdded.cursor = 0
		m.recentlyAdded.scrollOffset = 0
		if m.recentlyAdded.filter.Text != "" {
			m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
		}
		m.statusBar.notification = newNotification(sortNotification(m.recentlyAdded.sortState))
		return m, nil, true

	case viewHistory:
		next := CycleSortField(m.history.sortState.Field, historySortFields)
		m.history.sortState.Field = next
		if next == SortDefault {
			m.history.sortState.Dir = SortAsc
		}
		m.history = m.history.recomputeSortedTracks()
		m.history.cursor = 0
		m.history.scrollOffset = 0
		if m.history.filter.Text != "" {
			m.history = m.history.rebuildFilterCache()
		}
		m.statusBar.notification = newNotification(sortNotification(m.history.sortState))
		return m, nil, true

	case viewQueue:
		if m.queue.savingAsPlaylist {
			return m, nil, false
		}
		next := CycleSortField(m.queue.sortState.Field, queueSortFields)
		m.queue.sortState.Field = next
		if next == SortDefault {
			m.queue.sortState.Dir = SortAsc
		}
		m.queue = m.queue.recomputeDisplayOrder()
		m.queue.cursor = 0
		m.queue.scrollOffset = 0
		m.statusBar.notification = newNotification(sortNotification(m.queue.sortState))
		return m, nil, true

	case viewPlaylists:
		if m.playlists.creating || m.playlists.renaming {
			return m, nil, false
		}
		fields := playlistListSortFields
		if m.playlists.level == pltui.TrackLevel {
			fields = playlistTrackSortFields
		}
		next := CycleSortField(m.playlists.sortState.Field, fields)
		m.playlists.sortState.Field = next
		if next == SortDefault {
			m.playlists.sortState.Dir = SortAsc
		}
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = 0
		m.playlists.scrollOffset = 0
		m.statusBar.notification = newNotification(sortNotification(m.playlists.sortState))
		return m, nil, true

	case viewArtists:
		fields := artistsSortFieldsForLevel(m.artists.level)
		next := CycleSortField(m.artists.sortState.Field, fields)
		m.artists.sortState.Field = next
		if next == SortDefault {
			m.artists.sortState.Dir = SortAsc
			m = m.rederiveArtistsData()
		}
		m.artists = m.artists.recomputeArtistsSort()
		m.artists.cursor = 0
		m.artists.scrollOffset = 0
		m.statusBar.notification = newNotification(sortNotification(m.artists.sortState))
		return m, nil, true

	case viewAlbums:
		fields := albumsSortFieldsForLevel(m.albums.level)
		next := CycleSortField(m.albums.sortState.Field, fields)
		m.albums.sortState.Field = next
		if next == SortDefault {
			m.albums.sortState.Dir = SortAsc
			m = m.rederiveAlbumsData()
		}
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums.cursor = 0
		m.albums.scrollOffset = 0
		m.statusBar.notification = newNotification(sortNotification(m.albums.sortState))
		return m, nil, true

	case viewFavorites:
		fields := favoritesSortFieldsForLevel(m.favorites)
		if fields == nil {
			return m, nil, false
		}
		next := CycleSortField(m.favorites.sortState.Field, fields)
		m.favorites.sortState.Field = next
		if next == SortDefault {
			m.favorites.sortState.Dir = SortAsc
			m = m.rederiveFavoritesData()
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
		m.favorites.cursor = 0
		m.favorites.scrollOffset = 0
		m.favorites.selectedItems = nil
		m.statusBar.notification = newNotification(sortNotification(m.favorites.sortState))
		return m, nil, true

	case viewGenres:
		fields := genresSortFieldsForLevel(m.genres.level)
		next := CycleSortField(m.genres.sortState.Field, fields)
		m.genres.sortState.Field = next
		if next == SortDefault {
			m.genres.sortState.Dir = SortAsc
			m = m.rederiveGenresData()
		}
		m.genres = m.genres.recomputeGenresSort()
		m.genres.cursor = 0
		m.genres.scrollOffset = 0
		m.statusBar.notification = newNotification(sortNotification(m.genres.sortState))
		return m, nil, true

	case viewTags:
		fields := tagsSortFieldsForLevel(m.tagsView.level)
		if fields == nil {
			return m, nil, false
		}
		next := CycleSortField(m.tagsView.sortState.Field, fields)
		m.tagsView.sortState.Field = next
		if next == SortDefault {
			m.tagsView.sortState.Dir = SortAsc
		}
		m.tagsView = m.tagsView.recomputeTagsSort()
		m.tagsView.cursor = 0
		m.tagsView.scrollOffset = 0
		m.tagsView.selectedHashes = nil
		m.statusBar.notification = newNotification(sortNotification(m.tagsView.sortState))
		return m, nil, true

	default:
		return m, nil, false
	}
}

// handleSortDir sets the sort direction for the active view, preserving the
// focused item at its new position after re-sort.
func (m Model) handleSortDir(dir SortDir) (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		if m.library.sortState.Field == SortDefault {
			return m, nil, false
		}
		var focusedHash string
		if t, ok := m.library.view().FocusedTrack(); ok {
			focusedHash = t.Hash
		}
		m.library.sortState.Dir = dir
		m.library = m.library.recomputeSortedTracks()
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
		m.library.cursor = m.library.findTrackPosition(focusedHash)
		m.library.scrollOffset = adjustScroll(m.library.cursor, m.library.scrollOffset, layout.VisibleRows(m.library.height, 1))
		m.statusBar.notification = newNotification(sortNotification(m.library.sortState))
		return m, nil, true

	case viewRecentlyAdded:
		if m.recentlyAdded.sortState.Field == SortDefault {
			return m, nil, false
		}
		var focusedHash string
		if t, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			focusedHash = t.Hash
		}
		m.recentlyAdded.sortState.Dir = dir
		m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()
		if m.recentlyAdded.filter.Text != "" {
			m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
		}
		m.recentlyAdded.cursor = m.recentlyAdded.findTrackPosition(focusedHash)
		m.recentlyAdded.scrollOffset = adjustScroll(m.recentlyAdded.cursor, m.recentlyAdded.scrollOffset, layout.VisibleRows(m.recentlyAdded.height, 1))
		m.statusBar.notification = newNotification(sortNotification(m.recentlyAdded.sortState))
		return m, nil, true

	case viewHistory:
		if m.history.sortState.Field == SortDefault {
			return m, nil, false
		}
		var focusedHash string
		if t, ok := m.history.view().FocusedTrack(); ok {
			focusedHash = t.Hash
		}
		m.history.sortState.Dir = dir
		m.history = m.history.recomputeSortedTracks()
		if m.history.filter.Text != "" {
			m.history = m.history.rebuildFilterCache()
		}
		m.history.cursor = m.history.findTrackPosition(focusedHash)
		m.history.scrollOffset = adjustScroll(m.history.cursor, m.history.scrollOffset, layout.VisibleRows(m.history.height, 1))
		m.statusBar.notification = newNotification(sortNotification(m.history.sortState))
		return m, nil, true

	case viewQueue:
		if m.queue.sortState.Field == SortDefault {
			return m, nil, false
		}
		prevEntryIdx := m.queue.entryIndex(m.queue.cursor)
		m.queue.sortState.Dir = dir
		m.queue = m.queue.recomputeDisplayOrder()
		m.queue.cursor = m.queue.findEntryDisplayPosition(prevEntryIdx)
		m.queue.scrollOffset = adjustScroll(m.queue.cursor, m.queue.scrollOffset, m.queue.visibleRows())
		m.statusBar.notification = newNotification(sortNotification(m.queue.sortState))
		return m, nil, true

	case viewPlaylists:
		if m.playlists.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preservePlaylistCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.playlists.sortState))
		return m, nil, true

	case viewArtists:
		if m.artists.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveArtistsCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.artists.sortState))
		return m, nil, true

	case viewAlbums:
		if m.albums.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveAlbumsCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.albums.sortState))
		return m, nil, true

	case viewFavorites:
		if m.favorites.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveFavoritesCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.favorites.sortState))
		return m, nil, true

	case viewGenres:
		if m.genres.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveGenresCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.genres.sortState))
		return m, nil, true

	case viewTags:
		if m.tagsView.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveTagsCursorOnDirChange(dir)
		m.statusBar.notification = newNotification(sortNotification(m.tagsView.sortState))
		return m, nil, true

	default:
		return m, nil, false
	}
}

// sortNotification formats a transient notification string for the current sort state.
func sortNotification(s SortState) string {
	if s.Field == SortDefault {
		return "Sort: default"
	}
	return fmt.Sprintf("Sort: %s %s", SortFieldLabel(s.Field), SortDirIndicator(s.Dir))
}
