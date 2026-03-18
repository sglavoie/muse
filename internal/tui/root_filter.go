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
// not support filtering (e.g. Queue, Playlists).
func (m Model) allFilterOps() *filterOps {
	switch m.activeView {
	case viewLibrary:
		return &filterOps{
			deactivate: func() Model { m.library = m.library.DeactivateFilter(); return m },
			commit:     func() Model { m.library = m.library.CommitFilter(); return m },
			backspace:  func() Model { m.library = m.library.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.library = m.library.AppendFilterChar(ch); return m },
			isActive:   func() bool { return m.library.filter.Active },
			activate:   func() Model { m.library = m.library.ActivateFilter(); return m },
		}
	case viewArtists:
		return &filterOps{
			deactivate: func() Model { m.artists = m.artists.DeactivateFilter(); return m },
			commit:     func() Model { m.artists = m.artists.CommitFilter(); return m },
			backspace:  func() Model { m.artists = m.artists.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.artists = m.artists.AppendFilterChar(ch); return m },
			isActive:   func() bool { return m.artists.filter.Active },
			activate:   func() Model { m.artists = m.artists.ActivateFilter(); return m },
		}
	case viewAlbums:
		return &filterOps{
			deactivate: func() Model { m.albums = m.albums.DeactivateFilter(); return m },
			commit:     func() Model { m.albums = m.albums.CommitFilter(); return m },
			backspace:  func() Model { m.albums = m.albums.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.albums = m.albums.AppendFilterChar(ch); return m },
			isActive:   func() bool { return m.albums.filter.Active },
			activate:   func() Model { m.albums = m.albums.ActivateFilter(); return m },
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
