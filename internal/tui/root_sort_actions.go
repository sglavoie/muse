package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
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
	fields, state := m.activeSortFields()
	if len(fields) == 0 {
		return m, nil, false
	}
	return m.applySortField(sorting.CycleSortField(state.Field, fields))
}

// applySortField shares cache rebuilds and cursor resets with keyboard cycling.
func (m Model) applySortField(next sorting.SortField) (tea.Model, tea.Cmd, bool) {
	var ss sorting.SortState
	switch m.ActiveView {
	case viewLibrary:
		m.library, ss = m.library.ApplySortField(next)
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewQueue:
		if m.queue.SavingAsPlaylist {
			return m, nil, false
		}
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
		if m, ss, ok = m.applyBrowseSortField(next); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewFavorites:
		fields := favoritesSortFieldsForLevel(m.favorites)
		if fields == nil {
			return m, nil, false
		}
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
		if m.mixes.Level != 1 {
			return m, nil, false
		}
		m.mixes.TrackView, _ = m.mixes.TrackView.ApplySortField(next)
		m.mixes.TrackView.Cursor = 0
		m.mixes.TrackView.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.mixes.TrackView.SortState))
		return m, nil, true

	case viewInsights:
		if !m.insights.AtTracks() {
			return m, nil, false
		}
		m.insights.TrackView, _ = m.insights.TrackView.ApplySortField(next)
		m.insights.TrackView.Cursor = 0
		m.insights.TrackView.ScrollOffset = 0
		m.statusBar.NotifyInfo(sortNotification(m.insights.TrackView.SortState))
		return m, nil, true

	default:
		return m, nil, false
	}
}

// handleSortDir sets the sort direction for the active view, preserving the
// focused item at its new position after re-sort.
func (m Model) handleSortDir(dir sorting.SortDir) (tea.Model, tea.Cmd, bool) {
	switch m.ActiveView {
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
		if m.mixes.Level != 1 {
			return m, nil, false
		}
		var ss sorting.SortState
		var ok bool
		if m.mixes.TrackView, ss, ok = m.mixes.TrackView.ApplySortDir(dir); !ok {
			return m, nil, false
		}
		m.statusBar.NotifyInfo(sortNotification(ss))
		return m, nil, true

	case viewInsights:
		if !m.insights.AtTracks() {
			return m, nil, false
		}
		var ss sorting.SortState
		var ok bool
		if m.insights.TrackView, ss, ok = m.insights.TrackView.ApplySortDir(dir); !ok {
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
