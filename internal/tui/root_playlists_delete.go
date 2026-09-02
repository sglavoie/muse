package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// handlePlaylistDelete handles the 'd' key in the playlists view for deleting
// playlists (at list level) or removing tracks (at track level, including bulk).
func (m Model) handlePlaylistDelete() (Model, tea.Cmd) {
	if m.playlists.AtListLevel() {
		return m.handlePlaylistDeleteAtListLevel()
	}
	return m.handlePlaylistDeleteAtTrackLevel()
}

// handlePlaylistDeleteAtListLevel removes the focused playlist via soft-delete.
func (m Model) handlePlaylistDeleteAtListLevel() (Model, tea.Cmd) {
	effective := m.playlists.EffectivePlaylists()
	if len(effective) == 0 {
		return m, nil
	}
	displayIdx := m.playlists.Cursor
	if displayIdx < 0 || displayIdx >= len(effective) {
		return m, nil
	}
	entry := effective[displayIdx]
	canonicalIdx := -1
	for i, p := range m.playlists.Playlists {
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
	m.playlists.Playlists = append(m.playlists.Playlists[:canonicalIdx], m.playlists.Playlists[canonicalIdx+1:]...)
	m = m.preservePlaylistsSort()
	m.playlists.Cursor, m.playlists.ScrollOffset = layout.ClampAfterRemoval(
		m.playlists.Cursor, m.playlists.ScrollOffset, len(m.playlists.Playlists))
	return m, nil
}

// handlePlaylistDeleteAtTrackLevel removes one or more tracks from the active
// playlist. Bulk removal is used when a visual selection is active; otherwise
// the single focused track is removed. Smart playlists are read-only.
func (m Model) handlePlaylistDeleteAtTrackLevel() (Model, tea.Cmd) {
	if m.playlists.SmartQuery != nil {
		return m, nil
	}
	if m.playlists.HasTrackSelection() {
		return m.handlePlaylistDeleteBulkTracks()
	}
	return m.handlePlaylistDeleteSingleTrack()
}

// handlePlaylistDeleteBulkTracks removes all selected tracks from the active playlist.
func (m Model) handlePlaylistDeleteBulkTracks() (Model, tea.Cmd) {
	var removedEntries []db.PlaylistTrackRow
	var removedIndices []int
	for i, t := range m.playlists.Tracks {
		if _, ok := m.playlists.SelectedTrackIDs[t.ID]; ok {
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
	for _, t := range m.playlists.Tracks {
		if _, ok := m.playlists.SelectedTrackIDs[t.ID]; !ok {
			remaining = append(remaining, t)
		}
	}
	m.playlists.Tracks = remaining
	m = m.preservePlaylistsSort()
	m.playlists.Cursor, m.playlists.ScrollOffset = layout.ClampAfterRemoval(
		m.playlists.Cursor, m.playlists.ScrollOffset, len(m.playlists.Tracks))
	m = exitVisualMode(m)
	m.playlists = m.playlists.ClearTrackSelection()
	m.setUndoSlot(undoBulkRemovePlaylistTracksAction{entries: removedEntries, indices: removedIndices}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil
}

// handlePlaylistDeleteSingleTrack removes the focused track from the active playlist.
func (m Model) handlePlaylistDeleteSingleTrack() (Model, tea.Cmd) {
	if len(m.playlists.Tracks) == 0 {
		return m, nil
	}
	displayIdx := m.playlists.Cursor
	if displayIdx < 0 || displayIdx >= len(m.playlists.Tracks) {
		return m, nil
	}
	idx := m.playlists.TrackIndex(displayIdx)
	entry := m.playlists.Tracks[idx]
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
	m.playlists.Tracks = append(m.playlists.Tracks[:idx], m.playlists.Tracks[idx+1:]...)
	m = m.preservePlaylistsSort()
	m.playlists.Cursor, m.playlists.ScrollOffset = layout.ClampAfterRemoval(
		m.playlists.Cursor, m.playlists.ScrollOffset, len(m.playlists.Tracks))
	return m, nil
}
