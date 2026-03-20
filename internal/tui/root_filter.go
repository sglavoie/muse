package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
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

// allFilterOps returns the filter operations for the currently active view,
// regardless of whether the filter is active. Returns nil for views that do
// not support filtering (e.g. Playlists).
func (m Model) allFilterOps() *filterOps {
	switch m.activeView {
	case viewLibrary:
		return &filterOps{
			activate: func() Model {
				m.library.filter = m.library.filter.Activate()
				return m
			},
			commit: func() Model {
				m.library.filter = m.library.filter.Commit()
				return m
			},
			deactivate: func() Model {
				m.library.filter = m.library.filter.Deactivate()
				m.library.cursor = 0
				m.library.scrollOffset = 0
				m.library = m.library.rebuildFilterCache()
				return m
			},
			appendChar: func(ch rune) Model {
				var changed bool
				m.library.filter, changed = m.library.filter.AppendChar(ch)
				if changed {
					m.library.cursor = 0
					m.library.scrollOffset = 0
					m.library = m.library.rebuildFilterCache()
				}
				return m
			},
			backspace: func() Model {
				var changed bool
				m.library.filter, changed = m.library.filter.Backspace()
				if changed {
					m.library.cursor = 0
					m.library.scrollOffset = 0
					m.library = m.library.rebuildFilterCache()
				}
				return m
			},
			isActive: func() bool { return m.library.filter.Active },
		}
	case viewArtists:
		return &filterOps{
			activate: func() Model {
				m.artists.filter = m.artists.filter.Activate()
				return m
			},
			commit: func() Model {
				m.artists.filter = m.artists.filter.Commit()
				return m
			},
			deactivate: func() Model {
				m.artists.filter = m.artists.filter.Deactivate()
				m.artists.cursor = 0
				m.artists.scrollOffset = 0
				return m
			},
			appendChar: func(ch rune) Model {
				var changed bool
				m.artists.filter, changed = m.artists.filter.AppendChar(ch)
				if changed {
					m.artists.cursor = 0
					m.artists.scrollOffset = 0
				}
				return m
			},
			backspace: func() Model {
				var changed bool
				m.artists.filter, changed = m.artists.filter.Backspace()
				if changed {
					m.artists.cursor = 0
					m.artists.scrollOffset = 0
				}
				return m
			},
			isActive: func() bool { return m.artists.filter.Active },
		}
	case viewAlbums:
		return &filterOps{
			activate: func() Model {
				m.albums.filter = m.albums.filter.Activate()
				return m
			},
			commit: func() Model {
				m.albums.filter = m.albums.filter.Commit()
				return m
			},
			deactivate: func() Model {
				m.albums.filter = m.albums.filter.Deactivate()
				m.albums.cursor = 0
				m.albums.scrollOffset = 0
				return m
			},
			appendChar: func(ch rune) Model {
				var changed bool
				m.albums.filter, changed = m.albums.filter.AppendChar(ch)
				if changed {
					m.albums.cursor = 0
					m.albums.scrollOffset = 0
				}
				return m
			},
			backspace: func() Model {
				var changed bool
				m.albums.filter, changed = m.albums.filter.Backspace()
				if changed {
					m.albums.cursor = 0
					m.albums.scrollOffset = 0
				}
				return m
			},
			isActive: func() bool { return m.albums.filter.Active },
		}
	case viewQueue:
		return &filterOps{
			activate: func() Model {
				m.queue.filter = m.queue.filter.Activate()
				return m
			},
			commit: func() Model {
				m.queue.filter = m.queue.filter.Commit()
				return m
			},
			deactivate: func() Model {
				m.queue.filter = m.queue.filter.Deactivate()
				m.queue.cursor = 0
				m.queue.scrollOffset = 0
				return m
			},
			appendChar: func(ch rune) Model {
				var changed bool
				m.queue.filter, changed = m.queue.filter.AppendChar(ch)
				if changed {
					m.queue.cursor = 0
					m.queue.scrollOffset = 0
				}
				return m
			},
			backspace: func() Model {
				var changed bool
				m.queue.filter, changed = m.queue.filter.Backspace()
				if changed {
					m.queue.cursor = 0
					m.queue.scrollOffset = 0
				}
				return m
			},
			isActive: func() bool { return m.queue.filter.Active },
		}
	case viewFavorites:
		return &filterOps{
			activate: func() Model {
				m.favorites.filter = m.favorites.filter.Activate()
				return m
			},
			commit: func() Model {
				m.favorites.filter = m.favorites.filter.Commit()
				return m
			},
			deactivate: func() Model {
				m.favorites.filter = m.favorites.filter.Deactivate()
				m.favorites.cursor = 0
				m.favorites.scrollOffset = 0
				return m
			},
			appendChar: func(ch rune) Model {
				var changed bool
				m.favorites.filter, changed = m.favorites.filter.AppendChar(ch)
				if changed {
					m.favorites.cursor = 0
					m.favorites.scrollOffset = 0
				}
				return m
			},
			backspace: func() Model {
				var changed bool
				m.favorites.filter, changed = m.favorites.filter.Backspace()
				if changed {
					m.favorites.cursor = 0
					m.favorites.scrollOffset = 0
				}
				return m
			},
			isActive: func() bool { return m.favorites.filter.Active },
		}
	default:
		return nil
	}
}

// handleFilterBarMsg toggles the filter bar in the active view, or shows a
// transient notification if filtering is not supported in the current view.
func (m Model) handleFilterBarMsg() Model {
	ops := m.allFilterOps()
	if ops == nil {
		m.statusBar.notification = transientNotification{
			msg:       "Filter not available in this view",
			expiresAt: time.Now().Add(transientDuration),
		}
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
