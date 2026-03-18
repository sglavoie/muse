package tui

import tea "charm.land/bubbletea/v2"

// filterOps bundles the filter operations for a single view so that
// handleFilterKeyPress can work generically across library/artists/albums.
type filterOps struct {
	deactivate func() Model
	commit     func() Model
	backspace  func() Model
	appendChar func(rune) Model
}

// activeFilterOps returns the filter operations for the currently active view,
// or nil if no filter is active.
func (m Model) activeFilterOps() *filterOps {
	switch m.activeView {
	case viewLibrary:
		if !m.library.filter.Active {
			return nil
		}
		return &filterOps{
			deactivate: func() Model { m.library = m.library.DeactivateFilter(); return m },
			commit:     func() Model { m.library = m.library.CommitFilter(); return m },
			backspace:  func() Model { m.library = m.library.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.library = m.library.AppendFilterChar(ch); return m },
		}
	case viewArtists:
		if !m.artists.filter.Active {
			return nil
		}
		return &filterOps{
			deactivate: func() Model { m.artists = m.artists.DeactivateFilter(); return m },
			commit:     func() Model { m.artists = m.artists.CommitFilter(); return m },
			backspace:  func() Model { m.artists = m.artists.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.artists = m.artists.AppendFilterChar(ch); return m },
		}
	case viewAlbums:
		if !m.albums.filter.Active {
			return nil
		}
		return &filterOps{
			deactivate: func() Model { m.albums = m.albums.DeactivateFilter(); return m },
			commit:     func() Model { m.albums = m.albums.CommitFilter(); return m },
			backspace:  func() Model { m.albums = m.albums.BackspaceFilter(); return m },
			appendChar: func(ch rune) Model { m.albums = m.albums.AppendFilterChar(ch); return m },
		}
	default:
		return nil
	}
}

// handleFilterBarMsg toggles the filter bar in the active view.
func (m Model) handleFilterBarMsg() Model {
	switch m.activeView {
	case viewLibrary:
		if m.library.filter.Active {
			m.library = m.library.CommitFilter()
		} else {
			m.library = m.library.ActivateFilter()
		}
	case viewArtists:
		if m.artists.filter.Active {
			m.artists = m.artists.CommitFilter()
		} else {
			m.artists = m.artists.ActivateFilter()
		}
	case viewAlbums:
		if m.albums.filter.Active {
			m.albums = m.albums.CommitFilter()
		} else {
			m.albums = m.albums.ActivateFilter()
		}
	}
	return m
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
