package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// undoDeletePlaylistAction reverts a playlist soft-delete (undoKindDeletePlaylist).
type undoDeletePlaylistAction struct {
	entry db.PlaylistRow
	idx   int
}

func (a undoDeletePlaylistAction) kind() undoKind { return undoKindDeletePlaylist }

func (a undoDeletePlaylistAction) description() string {
	return "Playlist deleted — press u to undo"
}

func (a undoDeletePlaylistAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoDeletePlaylist(a.entry.ID); err != nil {
			slog.Warn("undo delete playlist failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.playlists.playlists))
	playlists := make([]db.PlaylistRow, len(m.playlists.playlists)+1)
	copy(playlists, m.playlists.playlists[:idx])
	playlists[idx] = a.entry
	copy(playlists[idx+1:], m.playlists.playlists[idx:])
	m.playlists.playlists = playlists
	m.playlists.cursor = idx
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Undone: restored playlist")
	return m, nil
}

// undoRemovePlaylistTrackAction reverts removal of a single playlist track
// (undoKindRemovePlaylistTrack).
type undoRemovePlaylistTrackAction struct {
	entry db.PlaylistTrackRow
	idx   int
}

func (a undoRemovePlaylistTrackAction) kind() undoKind { return undoKindRemovePlaylistTrack }

func (a undoRemovePlaylistTrackAction) description() string {
	return "Track removed — press u to undo"
}

func (a undoRemovePlaylistTrackAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoRemovePlaylistTrack(a.entry.ID); err != nil {
			slog.Warn("undo remove playlist track failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.playlists.tracks))
	tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
	copy(tracks, m.playlists.tracks[:idx])
	tracks[idx] = a.entry
	copy(tracks[idx+1:], m.playlists.tracks[idx:])
	m.playlists.tracks = tracks
	m.playlists.cursor = idx
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Undone: restored playlist track")
	return m, nil
}

// undoBulkRemovePlaylistTracksAction reverts removal of multiple selected
// playlist tracks (undoKindBulkRemovePlaylistTracks).
type undoBulkRemovePlaylistTracksAction struct {
	entries []db.PlaylistTrackRow
	indices []int
}

func (a undoBulkRemovePlaylistTracksAction) kind() undoKind {
	return undoKindBulkRemovePlaylistTracks
}

func (a undoBulkRemovePlaylistTracksAction) description() string {
	n := len(a.entries)
	return fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoBulkRemovePlaylistTracksAction) perform(m Model) (Model, tea.Cmd) {
	if m.playlistDeleter != nil {
		for _, t := range a.entries {
			if err := m.playlistDeleter.UndoRemovePlaylistTrack(t.ID); err != nil {
				slog.Warn("undo bulk remove playlist track failed", "err", err)
			}
		}
	}
	for i := range a.indices {
		idx := min(a.indices[i], len(m.playlists.tracks))
		tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
		copy(tracks, m.playlists.tracks[:idx])
		tracks[idx] = a.entries[i]
		copy(tracks[idx+1:], m.playlists.tracks[idx:])
		m.playlists.tracks = tracks
	}
	if m.playlists.cursor < 0 {
		m.playlists.cursor = 0
	} else if m.playlists.cursor >= len(m.playlists.tracks) {
		m.playlists.cursor = len(m.playlists.tracks) - 1
	}
	m.playlists = m.playlists.ensureCursorVisible()
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d playlist %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}
