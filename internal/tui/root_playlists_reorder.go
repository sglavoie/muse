package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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
	if m.playlists.AtListLevel() || m.playlists.SmartQuery != nil {
		return m, nil
	}
	if m.playlists.SortedTrackOrder != nil {
		return m, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted") }
	}

	idx := m.playlists.Cursor
	n := len(m.playlists.Tracks)

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
		return m, func() tea.Msg {
			if err := reorderer.ReorderPlaylistTrack(playlistID, id, dir); err != nil {
				slog.Error("reorder playlist track failed", "id", id, "err", err)
			}
			return nil
		}
	}
	return m, nil
}
