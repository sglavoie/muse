package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// handlePlaylistMoveTrackUp swaps the focused track with the one above it in
// the active playlist and persists the change asynchronously.
func (m Model) handlePlaylistMoveTrackUp() (Model, tea.Cmd) {
	return m.shiftPlaylistTrack(-1)
}

// handlePlaylistMoveTrackDown swaps the focused track with the one below it in
// the active playlist and persists the change asynchronously.
func (m Model) handlePlaylistMoveTrackDown() (Model, tea.Cmd) {
	return m.shiftPlaylistTrack(+1)
}

// shiftPlaylistTrack moves the focused track one position in the given
// direction (-1 = up, +1 = down) and persists the swap asynchronously.
func (m Model) shiftPlaylistTrack(dir int) (Model, tea.Cmd) {
	if m.playlistOrderPending {
		m.statusBar.Notify("Saving playlist order — try again shortly")
		return m, nil
	}
	if m.playlists.AtListLevel() || m.playlists.SmartQuery != nil {
		return m, nil
	}
	if m.playlists.Filter.Text != "" {
		return m, func() tea.Msg { return statusbar.TransientNotifyMsg("Clear filter before reordering") }
	}
	if m.playlists.SortedTrackOrder != nil {
		return m, func() tea.Msg { return statusbar.TransientNotifyMsg("Cannot reorder while sorted") }
	}

	idx := m.playlists.Cursor
	n := len(m.playlists.Tracks)
	if idx < 0 || idx >= n {
		return m, nil
	}
	before := playlistOrderIDs(m.playlists.Tracks)
	m.playlists.Tracks = slices.Clone(m.playlists.Tracks)

	if dir < 0 {
		if idx <= 0 || n < 2 {
			return m, nil
		}
		m.playlists.Tracks[idx-1], m.playlists.Tracks[idx] = m.playlists.Tracks[idx], m.playlists.Tracks[idx-1]
		m.playlists.Cursor = idx - 1
		if m.playlists.Cursor < m.playlists.ScrollOffset {
			m.playlists.ScrollOffset = m.playlists.Cursor
		}
	} else {
		if idx >= n-1 || n < 2 {
			return m, nil
		}
		m.playlists.Tracks[idx], m.playlists.Tracks[idx+1] = m.playlists.Tracks[idx+1], m.playlists.Tracks[idx]
		m.playlists.Cursor = idx + 1
		reserve := 3
		rows := pltui.FallbackVisibleRows
		if m.playlists.Height > 0 {
			rows = max(1, m.playlists.Height-reserve)
		}
		if m.playlists.Cursor >= m.playlists.ScrollOffset+rows {
			m.playlists.ScrollOffset = m.playlists.Cursor - rows + 1
		}
	}

	m = exitVisualMode(m)
	entry := m.playlists.Tracks[m.playlists.Cursor]
	playlistID := m.playlists.PendingLoadID
	if reorderer, ok := m.playlistDeleter.(playlistTrackReorderer); ok && playlistID != 0 {
		id := entry.ID
		after := playlistOrderIDs(m.playlists.Tracks)
		m.playlistOrderPending = true
		return m, func() tea.Msg {
			if err := reorderer.ReorderPlaylistTrack(playlistID, id, dir); err != nil {
				return playlistOrderSavedMsg{playlistID: playlistID, order: before, err: err}
			}
			return playlistOrderSavedMsg{playlistID: playlistID, order: after}
		}
	}
	return m, nil
}
