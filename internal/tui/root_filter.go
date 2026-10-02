package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/browse"
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
	switch m.ActiveView {
	case viewLibrary:
		return makeFilterOps(&m, &m.library.Filter, &m.library.Cursor, &m.library.ScrollOffset,
			func() { m.library = m.library.RebuildFilterCache() })
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return makeFilterOps(&m, &m.browse.Artists.Filter, &m.browse.Artists.Cursor, &m.browse.Artists.ScrollOffset, nil)
		case browse.BrowseModeAlbums:
			return makeFilterOps(&m, &m.browse.Albums.Filter, &m.browse.Albums.Cursor, &m.browse.Albums.ScrollOffset, nil)
		case browse.BrowseModeGenres:
			return makeFilterOps(&m, &m.browse.Genres.Filter, &m.browse.Genres.Cursor, &m.browse.Genres.ScrollOffset, nil)
		}
		return nil
	case viewQueue:
		return makeFilterOps(&m, &m.queue.Filter, &m.queue.Cursor, &m.queue.ScrollOffset, nil)
	case viewFavorites:
		return makeFilterOps(&m, &m.favorites.Filter, &m.favorites.Cursor, &m.favorites.ScrollOffset,
			func() { m.favorites.SelectedItems = nil })
	case viewTags:
		return makeFilterOps(&m, &m.tagsView.Filter, &m.tagsView.Cursor, &m.tagsView.ScrollOffset, nil)
	case viewMixes:
		if m.mixes.Level != 1 {
			return nil
		}
		return makeFilterOps(&m, &m.mixes.TrackView.Filter,
			&m.mixes.TrackView.Cursor,
			&m.mixes.TrackView.ScrollOffset,
			func() { m.mixes.TrackView = m.mixes.TrackView.RebuildFilterCache() })
	case viewInsights:
		if !m.insights.AtTracks() {
			return nil
		}
		return makeFilterOps(&m, &m.insights.TrackView.Filter,
			&m.insights.TrackView.Cursor,
			&m.insights.TrackView.ScrollOffset,
			func() { m.insights.TrackView = m.insights.TrackView.RebuildFilterCache() })
	default:
		return nil
	}
}

// handleFilterBarMsg toggles the filter bar in the active view, or shows a
// transient notification if filtering is not supported in the current view.
func (m Model) handleFilterBarMsg() (Model, tea.Cmd) {
	ops := m.allFilterOps()
	if ops == nil {
		if m.ActiveView == viewMixes {
			// The Mixes footer advertises "/" at both levels, but only an
			// open mix has a track list to filter.
			m.statusBar.Notify("Open a mix to filter its tracks")
			return m, nil
		}
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
		return m.markQuitting(), tea.Quit, true
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
