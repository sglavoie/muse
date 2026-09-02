package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// handleSortAction handles sort-related key actions. Returns (model, cmd, true)
// when the action was handled, or (zero, nil, false) to let the caller continue
// the handler chain.
func (m Model) handleSortAction(a keys.Action) (tea.Model, tea.Cmd, bool) {
	switch a {
	case keys.ActionSortCycle:
		m = exitVisualMode(m)
		return m.handleSortCycle()
	case keys.ActionSortAsc:
		m = exitVisualMode(m)
		return m.handleSortDir(sorting.SortAsc)
	case keys.ActionSortDesc:
		m = exitVisualMode(m)
		return m.handleSortDir(sorting.SortDesc)
	default:
		return m, nil, false
	}
}

// handleSortCycle cycles to the next sort field for the active view.
func (m Model) handleSortCycle() (tea.Model, tea.Cmd, bool) {
	var ss sorting.SortState
	switch m.activeView {
	case viewLibrary:
		switch m.library.Mode {
		case libtui.LibraryModeRecent:
			m.library, ss = m.library.CycleSortField(sorting.RecentlyAddedSortFields)
		case libtui.LibraryModeHistory:
			m.library, ss = m.library.CycleSortField(sorting.HistorySortFields)
		default:
			m.library, ss = m.library.CycleSortField(sorting.LibrarySortFields)
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewQueue:
		if m.queue.SavingAsPlaylist {
			return m, nil, false
		}
		next := sorting.CycleSortField(m.queue.SortState.Field, sorting.QueueSortFields)
		m.queue.SortState.Field = next
		if next == sorting.SortDefault {
			m.queue.SortState.Dir = sorting.SortAsc
		}
		m.queue = m.queue.RecomputeDisplayOrder()
		m.queue.Cursor = 0
		m.queue.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.queue.SortState))
		return m, nil, true

	case viewPlaylists:
		if m.playlists.Creating || m.playlists.Renaming {
			return m, nil, false
		}
		fields := sorting.PlaylistListSortFields
		if m.playlists.Level == pltui.TrackLevel {
			fields = sorting.PlaylistTrackSortFields
		}
		next := sorting.CycleSortField(m.playlists.SortState.Field, fields)
		m.playlists.SortState.Field = next
		if next == sorting.SortDefault {
			m.playlists.SortState.Dir = sorting.SortAsc
		}
		m.playlists = m.playlists.RecomputePlaylistSort()
		m.playlists.Cursor = 0
		m.playlists.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.playlists.SortState))
		return m, nil, true

	case viewBrowse:
		var ss sorting.SortState
		var ok bool
		if m, ss, ok = m.cycleBrowseSortField(); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewFavorites:
		fields := favoritesSortFieldsForLevel(m.favorites)
		if fields == nil {
			return m, nil, false
		}
		next := sorting.CycleSortField(m.favorites.SortState.Field, fields)
		m.favorites.SortState.Field = next
		if next == sorting.SortDefault {
			m.favorites.SortState.Dir = sorting.SortAsc
			m = m.rederiveFavoritesData()
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
		m.favorites.Cursor = 0
		m.favorites.ScrollOffset = 0
		m.favorites.SelectedItems = nil
		m.statusBar.NotifyInfo(sortNotification(m.favorites.SortState))
		return m, nil, true

	case viewTags:
		fields := tagsSortFieldsForLevel(m.tagsView.Level)
		if fields == nil {
			return m, nil, false
		}
		next := sorting.CycleSortField(m.tagsView.SortState.Field, fields)
		m.tagsView.SortState.Field = next
		if next == sorting.SortDefault {
			m.tagsView.SortState.Dir = sorting.SortAsc
		}
		m.tagsView = m.tagsView.RecomputeTagsSort()
		m.tagsView.Cursor = 0
		m.tagsView.ScrollOffset = 0
		m.tagsView.SelectedHashes = nil
		m.statusBar.NotifyInfo(sortNotification(m.tagsView.SortState))
		return m, nil, true

	case viewMixes:
		if m.mixes.level != 1 {
			return m, nil, false
		}
		m.mixes.trackView, _ = m.mixes.trackView.CycleSortField(sorting.LibrarySortFields)
		m.mixes.trackView.Cursor = 0
		m.mixes.trackView.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.mixes.trackView.SortState))
		return m, nil, true

	case viewInsights:
		if !m.insights.atTracks() {
			return m, nil, false
		}
		m.insights.trackView, _ = m.insights.trackView.CycleSortField(sorting.LibrarySortFields)
		m.insights.trackView.Cursor = 0
		m.insights.trackView.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.insights.trackView.SortState))
		return m, nil, true

	default:
		return m, nil, false
	}
}

// handleSortDir sets the sort direction for the active view, preserving the
// focused item at its new position after re-sort.
func (m Model) handleSortDir(dir sorting.SortDir) (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		var ss sorting.SortState
		var ok bool
		if m.library, ss, ok = m.library.ApplySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewQueue:
		if m.queue.SortState.Field == sorting.SortDefault {
			return m, nil, false
		}
		prevEntryIdx := m.queue.FocusedEntryIndex()
		m.queue.SortState.Dir = dir
		m.queue = m.queue.RecomputeDisplayOrder()
		m.queue.Cursor = m.queue.FindEntryFilteredPosition(prevEntryIdx)
		m.queue.ScrollOffset = sorting.AdjustScroll(m.queue.Cursor, m.queue.ScrollOffset, m.queue.VisibleRows())
		m.statusBar.NotifyInfo(sortNotification(m.queue.SortState))
		return m, nil, true

	case viewPlaylists:
		if m.playlists.SortState.Field == sorting.SortDefault {
			return m, nil, false
		}
		m = m.preservePlaylistCursorOnDirChange(dir)
		m.statusBar.NotifyInfo(sortNotification(m.playlists.SortState))
		return m, nil, true

	case viewBrowse:
		var ss sorting.SortState
		var ok bool
		if m, ss, ok = m.shiftBrowseSortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewFavorites:
		if m.favorites.SortState.Field == sorting.SortDefault {
			return m, nil, false
		}
		m = m.preserveFavoritesCursorOnDirChange(dir)
		m.statusBar.NotifyInfo(sortNotification(m.favorites.SortState))
		return m, nil, true

	case viewTags:
		if m.tagsView.SortState.Field == sorting.SortDefault {
			return m, nil, false
		}
		m = m.preserveTagsCursorOnDirChange(dir)
		m.statusBar.NotifyInfo(sortNotification(m.tagsView.SortState))
		return m, nil, true

	case viewMixes:
		if m.mixes.level != 1 {
			return m, nil, false
		}
		var ss sorting.SortState
		var ok bool
		if m.mixes.trackView, ss, ok = m.mixes.trackView.ApplySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewInsights:
		if !m.insights.atTracks() {
			return m, nil, false
		}
		var ss sorting.SortState
		var ok bool
		if m.insights.trackView, ss, ok = m.insights.trackView.ApplySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	default:
		return m, nil, false
	}
}

// sortNotification formats a transient notification string for the current sort state.
func sortNotification(s sorting.SortState) string {
	if s.Field == sorting.SortDefault {
		return "Sort: default"
	}
	return fmt.Sprintf("Sort: %s %s", sorting.SortFieldLabel(s.Field), sorting.SortDirIndicator(s.Dir))
}
