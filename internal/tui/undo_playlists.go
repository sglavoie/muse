package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/text"
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

func (a undoDeletePlaylistAction) perform(m Model) (Model, error) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoDeletePlaylist(a.entry.ID); err != nil {
			return m, fmt.Errorf("restore playlist: %w", err)
		}
	}
	idx := min(a.idx, len(m.playlists.Playlists))
	playlists := make([]db.PlaylistRow, len(m.playlists.Playlists)+1)
	copy(playlists, m.playlists.Playlists[:idx])
	playlists[idx] = a.entry
	copy(playlists[idx+1:], m.playlists.Playlists[idx:])
	m.playlists.Playlists = playlists
	m = m.resetPlaylistsSort()
	m.playlists.Cursor = sorting.FindPlaylistPosition(m.playlists.EffectivePlaylists(), a.entry.ID)
	m.playlists = m.playlists.EnsureCursorVisible()
	m.statusBar.Notify("Undone: restored playlist")
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

func (a undoRemovePlaylistTrackAction) perform(m Model) (Model, error) {
	if m.playlistDeleter != nil {
		if err := m.playlistDeleter.UndoRemovePlaylistTrack(a.entry.ID); err != nil {
			return m, fmt.Errorf("restore playlist track: %w", err)
		}
	}
	idx := min(a.idx, len(m.playlists.Tracks))
	tracks := make([]db.PlaylistTrackRow, len(m.playlists.Tracks)+1)
	copy(tracks, m.playlists.Tracks[:idx])
	tracks[idx] = a.entry
	copy(tracks[idx+1:], m.playlists.Tracks[idx:])
	m.playlists.Tracks = tracks
	m = m.resetPlaylistsSort()
	m.playlists.Cursor = m.playlists.FindTrackDisplayPosition(idx)
	m.playlists = m.playlists.EnsureCursorVisible()
	m.statusBar.Notify("Undone: restored playlist track")
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
	return fmt.Sprintf("%d %s removed — press u to undo", n, text.Pluralize(n, "track", "tracks"))
}

func (a undoBulkRemovePlaylistTracksAction) perform(m Model) (Model, error) {
	if m.playlistDeleter != nil {
		ids := make([]int64, len(a.entries))
		for i, t := range a.entries {
			ids[i] = t.ID
		}
		if err := m.playlistDeleter.UndoRemovePlaylistTracks(ids); err != nil {
			return m, fmt.Errorf("restore playlist tracks: %w", err)
		}
	}
	for i := range a.indices {
		idx := min(a.indices[i], len(m.playlists.Tracks))
		tracks := make([]db.PlaylistTrackRow, len(m.playlists.Tracks)+1)
		copy(tracks, m.playlists.Tracks[:idx])
		tracks[idx] = a.entries[i]
		copy(tracks[idx+1:], m.playlists.Tracks[idx:])
		m.playlists.Tracks = tracks
	}
	m = m.resetPlaylistsSort()
	if m.playlists.Cursor < 0 {
		m.playlists.Cursor = 0
	} else if m.playlists.Cursor >= len(m.playlists.Tracks) {
		m.playlists.Cursor = len(m.playlists.Tracks) - 1
	}
	m.playlists = m.playlists.EnsureCursorVisible()
	n := len(a.entries)
	m.statusBar.Notify(fmt.Sprintf("Undone: restored %d playlist %s", n, text.Pluralize(n, "track", "tracks")))
	return m, nil
}
