package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
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
	var ss SortState
	switch m.activeView {
	case viewLibrary:
		switch m.library.mode {
		case LibraryModeRecent:
			m.library, ss = m.library.cycleSortField(recentlyAddedSortFields)
		case LibraryModeHistory:
			m.library, ss = m.library.cycleSortField(historySortFields)
		default:
			m.library, ss = m.library.cycleSortField(librarySortFields)
		}
		m.statusBar.notifyInfo(sortNotification(ss))
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
		m.statusBar.notifyInfo(sortNotification(m.queue.sortState))
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
		m.statusBar.notifyInfo(sortNotification(m.playlists.sortState))
		return m, nil, true

	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			fields := artistsSortFieldsForLevel(m.browse.artists.level)
			next := CycleSortField(m.browse.artists.sortState.Field, fields)
			m.browse.artists.sortState.Field = next
			if next == SortDefault {
				m.browse.artists.sortState.Dir = SortAsc
				m = m.rederiveArtistsData()
			}
			m.browse.artists = m.browse.artists.recomputeArtistsSort()
			m.browse.artists.cursor = 0
			m.browse.artists.scrollOffset = 0
			m.statusBar.notifyInfo(sortNotification(m.browse.artists.sortState))
			return m, nil, true
		case BrowseModeAlbums:
			fields := albumsSortFieldsForLevel(m.browse.albums.level)
			next := CycleSortField(m.browse.albums.sortState.Field, fields)
			m.browse.albums.sortState.Field = next
			if next == SortDefault {
				m.browse.albums.sortState.Dir = SortAsc
				m = m.rederiveAlbumsData()
			}
			m.browse.albums = m.browse.albums.recomputeAlbumsSort()
			m.browse.albums.cursor = 0
			m.browse.albums.scrollOffset = 0
			m.statusBar.notifyInfo(sortNotification(m.browse.albums.sortState))
			return m, nil, true
		case BrowseModeGenres:
			fields := genresSortFieldsForLevel(m.browse.genres.level)
			next := CycleSortField(m.browse.genres.sortState.Field, fields)
			m.browse.genres.sortState.Field = next
			if next == SortDefault {
				m.browse.genres.sortState.Dir = SortAsc
				m = m.rederiveGenresData()
			}
			m.browse.genres = m.browse.genres.recomputeGenresSort()
			m.browse.genres.cursor = 0
			m.browse.genres.scrollOffset = 0
			m.statusBar.notifyInfo(sortNotification(m.browse.genres.sortState))
			return m, nil, true
		}
		return m, nil, false

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
		m.statusBar.notifyInfo(sortNotification(m.favorites.sortState))
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
		m.statusBar.notifyInfo(sortNotification(m.tagsView.sortState))
		return m, nil, true

	case viewMixes:
		if m.mixes.level != 1 {
			return m, nil, false
		}
		m.mixes.trackView, _ = m.mixes.trackView.cycleSortField(librarySortFields)
		m.mixes.trackView.cursor = 0
		m.mixes.trackView.scrollOffset = 0
		m.statusBar.notifyInfo(sortNotification(m.mixes.trackView.sortState))
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
		var ss SortState
		var ok bool
		if m.library, ss, ok = m.library.applySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.notifyInfo(sortNotification(ss))
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
		m.statusBar.notifyInfo(sortNotification(m.queue.sortState))
		return m, nil, true

	case viewPlaylists:
		if m.playlists.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preservePlaylistCursorOnDirChange(dir)
		m.statusBar.notifyInfo(sortNotification(m.playlists.sortState))
		return m, nil, true

	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.sortState.Field == SortDefault {
				return m, nil, false
			}
			m = m.preserveArtistsCursorOnDirChange(dir)
			m.statusBar.notifyInfo(sortNotification(m.browse.artists.sortState))
			return m, nil, true
		case BrowseModeAlbums:
			if m.browse.albums.sortState.Field == SortDefault {
				return m, nil, false
			}
			m = m.preserveAlbumsCursorOnDirChange(dir)
			m.statusBar.notifyInfo(sortNotification(m.browse.albums.sortState))
			return m, nil, true
		case BrowseModeGenres:
			if m.browse.genres.sortState.Field == SortDefault {
				return m, nil, false
			}
			m = m.preserveGenresCursorOnDirChange(dir)
			m.statusBar.notifyInfo(sortNotification(m.browse.genres.sortState))
			return m, nil, true
		}
		return m, nil, false

	case viewFavorites:
		if m.favorites.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveFavoritesCursorOnDirChange(dir)
		m.statusBar.notifyInfo(sortNotification(m.favorites.sortState))
		return m, nil, true

	case viewTags:
		if m.tagsView.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveTagsCursorOnDirChange(dir)
		m.statusBar.notifyInfo(sortNotification(m.tagsView.sortState))
		return m, nil, true

	case viewMixes:
		if m.mixes.level != 1 {
			return m, nil, false
		}
		var ss SortState
		var ok bool
		if m.mixes.trackView, ss, ok = m.mixes.trackView.applySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.notifyInfo(sortNotification(ss))
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
