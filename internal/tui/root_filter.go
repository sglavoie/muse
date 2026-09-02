package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/filter"
)

// filterOps bundles the filter operations for a single view so that
// handleFilterKeyPress and handleFilterBarMsg can work generically across
// library/artists/albums.
type filterOps struct {
	deactivate func() Model
	commit     func() Model
	update     func(tea.KeyPressMsg) (Model, tea.Cmd)
	updateMsg  func(tea.Msg) (Model, tea.Cmd)
	isActive   func() bool
	activate   func() (Model, tea.Cmd)
}

// activeFilterOps returns the filter operations for the currently active view,
// or nil if no filter is active.
func (m Model) activeFilterOps() *filterOps {
	ops := m.allFilterOps()
	if ops == nil || !ops.isActive() {
		return nil
	}
	return ops
}

// makeFilterOps builds a filterOps for a single view using pointers into the
// model copy. f, cursor, and scrollOffset point into m's fields so that
// mutations are visible when the closures return *m. rebuild is called after
// any operation that changes the filter text (deactivate, update with text
// change); pass nil if no cache rebuild is needed.
func makeFilterOps(m *Model, f *filter.State, cursor *int, scrollOffset *int, rebuild func()) *filterOps {
	return &filterOps{
		activate: func() (Model, tea.Cmd) {
			var cmd tea.Cmd
			*f, cmd = f.Activate()
			return *m, cmd
		},
		commit: func() Model {
			*f = f.Commit()
			return *m
		},
		deactivate: func() Model {
			*f = f.Deactivate()
			*cursor = 0
			*scrollOffset = 0
			if rebuild != nil {
				rebuild()
			}
			return *m
		},
		update: func(msg tea.KeyPressMsg) (Model, tea.Cmd) {
			oldText := f.Text
			var cmd tea.Cmd
			*f, cmd = f.Update(msg)
			if f.Text != oldText {
				*cursor = 0
				*scrollOffset = 0
				if rebuild != nil {
					rebuild()
				}
			}
			return *m, cmd
		},
		updateMsg: func(msg tea.Msg) (Model, tea.Cmd) {
			oldText := f.Text
			var cmd tea.Cmd
			*f, cmd = f.UpdateMsg(msg)
			if f.Text != oldText {
				*cursor = 0
				*scrollOffset = 0
				if rebuild != nil {
					rebuild()
				}
			}
			return *m, cmd
		},
		isActive: func() bool { return f.Active },
	}
}

// allFilterOps returns the filter operations for the currently active view,
// regardless of whether the filter is active. Returns nil for views that do
// not support filtering (e.g. Playlists).
func (m Model) allFilterOps() *filterOps {
	switch m.activeView {
	case viewLibrary:
		return makeFilterOps(&m, &m.library.filter, &m.library.cursor, &m.library.scrollOffset,
			func() { m.library = m.library.rebuildFilterCache() })
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return makeFilterOps(&m, &m.browse.artists.filter, &m.browse.artists.cursor, &m.browse.artists.scrollOffset, nil)
		case BrowseModeAlbums:
			return makeFilterOps(&m, &m.browse.albums.filter, &m.browse.albums.cursor, &m.browse.albums.scrollOffset, nil)
		case BrowseModeGenres:
			return makeFilterOps(&m, &m.browse.genres.filter, &m.browse.genres.cursor, &m.browse.genres.scrollOffset, nil)
		}
		return nil
	case viewQueue:
		return makeFilterOps(&m, &m.queue.filter, &m.queue.cursor, &m.queue.scrollOffset, nil)
	case viewFavorites:
		return makeFilterOps(&m, &m.favorites.filter, &m.favorites.cursor, &m.favorites.scrollOffset,
			func() { m.favorites.selectedItems = nil })
	case viewTags:
		return makeFilterOps(&m, &m.tagsView.filter, &m.tagsView.cursor, &m.tagsView.scrollOffset, nil)
	case viewMixes:
		if m.mixes.level != 1 {
			return nil
		}
		return makeFilterOps(&m, &m.mixes.trackView.filter,
			&m.mixes.trackView.cursor,
			&m.mixes.trackView.scrollOffset,
			func() { m.mixes.trackView = m.mixes.trackView.rebuildFilterCache() })
	case viewInsights:
		if !m.insights.atTracks() {
			return nil
		}
		return makeFilterOps(&m, &m.insights.trackView.filter,
			&m.insights.trackView.cursor,
			&m.insights.trackView.scrollOffset,
			func() { m.insights.trackView = m.insights.trackView.rebuildFilterCache() })
	default:
		return nil
	}
}

// handleFilterBarMsg toggles the filter bar in the active view, or shows a
// transient notification if filtering is not supported in the current view.
func (m Model) handleFilterBarMsg() (Model, tea.Cmd) {
	ops := m.allFilterOps()
	if ops == nil {
		m.statusBar.Notify("Filter not available in this view")
		return m, nil
	}
	if ops.isActive() {
		return ops.commit(), nil
	}
	// Activating the filter changes the visible list, so exit visual mode.
	result, cmd := ops.activate()
	result = exitVisualMode(result)
	return result, cmd
}

// handleFilterKeyPress intercepts keypresses when a filter bar is active in
// the library, artists, or albums view. It returns true in the third value
// when the keypress was consumed by a filter bar.
func (m Model) handleFilterKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	ops := m.activeFilterOps()
	if ops == nil {
		return m, nil, false
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m = ops.deactivate()
	case "enter", "/":
		m = ops.commit()
	default:
		var cmd tea.Cmd
		m, cmd = ops.update(msg)
		return m, cmd, true
	}
	return m, nil, true
}
