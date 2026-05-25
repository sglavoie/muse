package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
)

// handlePlaylistDelete handles the 'd' key in the playlists view for deleting
// playlists (at list level) or removing tracks (at track level, including bulk).
func (m Model) handlePlaylistDelete() (Model, tea.Cmd) {
	if m.playlists.atListLevel() {
		return m.handlePlaylistDeleteAtListLevel()
	}
	return m.handlePlaylistDeleteAtTrackLevel()
}

// handlePlaylistDeleteAtListLevel removes the focused playlist via soft-delete.
func (m Model) handlePlaylistDeleteAtListLevel() (Model, tea.Cmd) {
	effective := m.playlists.effectivePlaylists()
	if len(effective) == 0 {
		return m, nil
	}
	displayIdx := m.playlists.cursor
	if displayIdx < 0 || displayIdx >= len(effective) {
		return m, nil
	}
	entry := effective[displayIdx]
	canonicalIdx := -1
	for i, p := range m.playlists.playlists {
		if p.ID == entry.ID {
			canonicalIdx = i
			break
		}
	}
	if canonicalIdx < 0 {
		return m, nil
	}
	pd := m.playlistDeleter
	cancel := m.scheduleSoftDelete(
		func() {
			if pd != nil {
				if err := pd.DeletePlaylistSoft(entry.ID); err != nil {
					slog.Warn("playlist soft-delete failed", "err", err)
				}
			}
		},
		func(before int64) {
			if pd != nil {
				if err := pd.PurgeDeletedPlaylists(before); err != nil {
					slog.Warn("playlist purge failed", "err", err)
				}
			}
		},
	)
	m.setUndoSlot(undoDeletePlaylistAction{entry: entry, idx: canonicalIdx}, cancel)
	player.PlaySound(player.SoundRemoved)
	m.playlists.playlists = append(m.playlists.playlists[:canonicalIdx], m.playlists.playlists[canonicalIdx+1:]...)
	m = m.preservePlaylistsSort()
	m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
		m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.playlists))
	return m, nil
}

// handlePlaylistDeleteAtTrackLevel removes one or more tracks from the active
// playlist. Bulk removal is used when a visual selection is active; otherwise
// the single focused track is removed. Smart playlists are read-only.
func (m Model) handlePlaylistDeleteAtTrackLevel() (Model, tea.Cmd) {
	if m.playlists.smartQuery != nil {
		return m, nil
	}
	if m.playlists.hasTrackSelection() {
		return m.handlePlaylistDeleteBulkTracks()
	}
	return m.handlePlaylistDeleteSingleTrack()
}

// handlePlaylistDeleteBulkTracks removes all selected tracks from the active playlist.
func (m Model) handlePlaylistDeleteBulkTracks() (Model, tea.Cmd) {
	var removedEntries []db.PlaylistTrackRow
	var removedIndices []int
	for i, t := range m.playlists.tracks {
		if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
			removedEntries = append(removedEntries, t)
			removedIndices = append(removedIndices, i)
		}
	}
	pd := m.playlistDeleter
	cancel := m.scheduleSoftDelete(
		func() {
			if pd != nil {
				for _, t := range removedEntries {
					if err := pd.RemovePlaylistTrackSoft(t.ID); err != nil {
						slog.Warn("playlist track soft-delete failed", "err", err)
					}
				}
			}
		},
		func(before int64) {
			if pd != nil {
				if err := pd.PurgeDeletedPlaylistTracks(before); err != nil {
					slog.Warn("playlist track purge failed", "err", err)
				}
			}
		},
	)
	var remaining []db.PlaylistTrackRow
	for _, t := range m.playlists.tracks {
		if _, ok := m.playlists.selectedTrackIDs[t.ID]; !ok {
			remaining = append(remaining, t)
		}
	}
	m.playlists.tracks = remaining
	m = m.preservePlaylistsSort()
	m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
		m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
	m = exitVisualMode(m)
	m.playlists = m.playlists.clearTrackSelection()
	m.setUndoSlot(undoBulkRemovePlaylistTracksAction{entries: removedEntries, indices: removedIndices}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil
}

// handlePlaylistDeleteSingleTrack removes the focused track from the active playlist.
func (m Model) handlePlaylistDeleteSingleTrack() (Model, tea.Cmd) {
	if len(m.playlists.tracks) == 0 {
		return m, nil
	}
	displayIdx := m.playlists.cursor
	if displayIdx < 0 || displayIdx >= len(m.playlists.tracks) {
		return m, nil
	}
	idx := m.playlists.trackIndex(displayIdx)
	entry := m.playlists.tracks[idx]
	pd := m.playlistDeleter
	cancel := m.scheduleSoftDelete(
		func() {
			if pd != nil {
				if err := pd.RemovePlaylistTrackSoft(entry.ID); err != nil {
					slog.Warn("playlist track soft-delete failed", "err", err)
				}
			}
		},
		func(before int64) {
			if pd != nil {
				if err := pd.PurgeDeletedPlaylistTracks(before); err != nil {
					slog.Warn("playlist track purge failed", "err", err)
				}
			}
		},
	)
	m.setUndoSlot(undoRemovePlaylistTrackAction{entry: entry, idx: idx}, cancel)
	player.PlaySound(player.SoundRemoved)
	m.playlists.tracks = append(m.playlists.tracks[:idx], m.playlists.tracks[idx+1:]...)
	m = m.preservePlaylistsSort()
	m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
		m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
	return m, nil
}
