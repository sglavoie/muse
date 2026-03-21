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
	backspace  func() Model
	appendChar func(rune) Model
	isActive   func() bool
	activate   func() Model
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
// any operation that changes the filter text (deactivate, appendChar,
// backspace); pass nil if no cache rebuild is needed.
func makeFilterOps(m *Model, f *filter.State, cursor *int, scrollOffset *int, rebuild func()) *filterOps {
	return &filterOps{
		activate: func() Model {
			*f = f.Activate()
			return *m
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
		appendChar: func(ch rune) Model {
			var changed bool
			*f, changed = f.AppendChar(ch)
			if changed {
				*cursor = 0
				*scrollOffset = 0
				if rebuild != nil {
					rebuild()
				}
			}
			return *m
		},
		backspace: func() Model {
			var changed bool
			*f, changed = f.Backspace()
			if changed {
				*cursor = 0
				*scrollOffset = 0
				if rebuild != nil {
					rebuild()
				}
			}
			return *m
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
	case viewArtists:
		return makeFilterOps(&m, &m.artists.filter, &m.artists.cursor, &m.artists.scrollOffset, nil)
	case viewAlbums:
		return makeFilterOps(&m, &m.albums.filter, &m.albums.cursor, &m.albums.scrollOffset, nil)
	case viewQueue:
		return makeFilterOps(&m, &m.queue.filter, &m.queue.cursor, &m.queue.scrollOffset, nil)
	case viewFavorites:
		return makeFilterOps(&m, &m.favorites.filter, &m.favorites.cursor, &m.favorites.scrollOffset, nil)
	default:
		return nil
	}
}

// handleFilterBarMsg toggles the filter bar in the active view, or shows a
// transient notification if filtering is not supported in the current view.
func (m Model) handleFilterBarMsg() Model {
	ops := m.allFilterOps()
	if ops == nil {
		m.statusBar.notification = newNotification("Filter not available in this view")
		return m
	}
	if ops.isActive() {
		return ops.commit()
	}
	return ops.activate()
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
	case "backspace":
		m = ops.backspace()
	default:
		if msg.Text != "" {
			for _, ch := range msg.Text {
				m = ops.appendChar(ch)
			}
		}
	}
	return m, nil, true
}
