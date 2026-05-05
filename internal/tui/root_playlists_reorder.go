package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
)

// handlePlaylistMoveTrackUp swaps the focused track with the one above it in
// the active playlist and persists the change asynchronously.
func (m Model) handlePlaylistMoveTrackUp() (Model, tea.Cmd) {
	if m.playlists.atListLevel() || m.playlists.smartQuery != nil {
		return m, nil
	}
	if m.playlists.sortedTrackOrder != nil {
		return m, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted") }
	}
	if m.playlists.cursor <= 0 || len(m.playlists.tracks) < 2 {
		return m, nil
	}
	idx := m.playlists.cursor
	m.playlists.tracks[idx-1], m.playlists.tracks[idx] = m.playlists.tracks[idx], m.playlists.tracks[idx-1]
	m.playlists.cursor = idx - 1
	if m.playlists.cursor < m.playlists.scrollOffset {
		m.playlists.scrollOffset = m.playlists.cursor
	}
	m = exitVisualMode(m)
	entry := m.playlists.tracks[m.playlists.cursor]
	playlistID := m.playlists.pendingLoadID
	if reorderer, ok := m.playlistDeleter.(playlistTrackReorderer); ok && playlistID != 0 {
		id := entry.ID
		return m, func() tea.Msg {
			if err := reorderer.ReorderPlaylistTrack(playlistID, id, -1); err != nil {
				slog.Error("reorder playlist track failed", "id", id, "err", err)
			}
			return nil
		}
	}
	return m, nil
}

// handlePlaylistMoveTrackDown swaps the focused track with the one below it in
// the active playlist and persists the change asynchronously.
func (m Model) handlePlaylistMoveTrackDown() (Model, tea.Cmd) {
	if m.playlists.atListLevel() || m.playlists.smartQuery != nil {
		return m, nil
	}
	if m.playlists.sortedTrackOrder != nil {
		return m, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted") }
	}
	if m.playlists.cursor >= len(m.playlists.tracks)-1 || len(m.playlists.tracks) < 2 {
		return m, nil
	}
	idx := m.playlists.cursor
	m.playlists.tracks[idx], m.playlists.tracks[idx+1] = m.playlists.tracks[idx+1], m.playlists.tracks[idx]
	m.playlists.cursor = idx + 1
	reserve := 3
	rows := playlistFallbackVisibleRows
	if m.playlists.height > 0 {
		rows = max(1, m.playlists.height-reserve)
	}
	if m.playlists.cursor >= m.playlists.scrollOffset+rows {
		m.playlists.scrollOffset = m.playlists.cursor - rows + 1
	}
	m = exitVisualMode(m)
	entry := m.playlists.tracks[m.playlists.cursor]
	playlistID := m.playlists.pendingLoadID
	if reorderer, ok := m.playlistDeleter.(playlistTrackReorderer); ok && playlistID != 0 {
		id := entry.ID
		return m, func() tea.Msg {
			if err := reorderer.ReorderPlaylistTrack(playlistID, id, +1); err != nil {
				slog.Error("reorder playlist track failed", "id", id, "err", err)
			}
			return nil
		}
	}
	return m, nil
}
