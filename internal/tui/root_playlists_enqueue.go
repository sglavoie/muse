package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// handlePlaylistEnqueueAll enqueues all tracks from the focused or open
// playlist into the play queue.
func (m Model) handlePlaylistEnqueueAll() (Model, tea.Cmd, bool) {
	if m.batchEnqueuer == nil {
		return m, nil, true
	}

	pid, playlistName, isSmart, smartQuery, ok := m.resolvePlaylistForEnqueue()
	if !ok {
		return m, nil, true
	}

	if isSmart && smartQuery != nil {
		return m.enqueueSmartPlaylist(*smartQuery, playlistName)
	}
	if m.playlistReader == nil {
		return m, nil, true
	}
	return m.enqueueRegularPlaylist(pid, playlistName)
}

// resolvePlaylistForEnqueue identifies the target playlist for enqueue-all,
// whether the user is at list level (focused playlist) or track level (open
// playlist). Returns false when no valid playlist can be determined.
func (m Model) resolvePlaylistForEnqueue() (pid int64, name string, isSmart bool, smartQuery *db.SmartPlaylistQuery, ok bool) {
	if m.playlists.AtListLevel() {
		pl, found := m.playlists.FocusedPlaylist()
		if !found {
			return 0, "", false, nil, false
		}
		if pl.IsSmart {
			q, err := db.ParseSmartPlaylistQuery(pl.Query)
			if err != nil || m.smartEval == nil {
				return 0, "", false, nil, false
			}
			return pl.ID, pl.Name, true, &q, true
		}
		return pl.ID, pl.Name, false, nil, true
	}

	if m.playlists.ListCursor < 0 || m.playlists.ListCursor >= len(m.playlists.Playlists) {
		return 0, "", false, nil, false
	}
	entry := m.playlists.Playlists[m.playlists.ListCursor]
	return entry.ID, entry.Name, m.playlists.SmartQuery != nil, m.playlists.SmartQuery, true
}

func (m Model) enqueueSmartPlaylist(q db.SmartPlaylistQuery, playlistName string) (Model, tea.Cmd, bool) {
	evaluator := m.smartEval
	be := m.batchEnqueuer
	wasEmpty := len(m.queue.entries) == 0
	return m, func() tea.Msg {
		rows, err := evaluator.EvalSmartPlaylist(q)
		if err != nil {
			return BatchEnqueuedMsg{Err: err}
		}
		if len(rows) == 0 {
			return BatchEnqueuedMsg{PlaylistName: playlistName}
		}
		tracks := playlistRowsToLibTracks(rows)
		ids, err := be.AppendTracks(tracks, playlistName)
		if err != nil {
			return BatchEnqueuedMsg{Err: err}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, playlistName),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: playlistName,
		}
	}, true
}

func (m Model) enqueueRegularPlaylist(pid int64, playlistName string) (Model, tea.Cmd, bool) {
	reader := m.playlistReader
	be := m.batchEnqueuer
	wasEmpty := len(m.queue.entries) == 0
	return m, func() tea.Msg {
		rows, err := reader.ReadPlaylistTracks(pid)
		if err != nil {
			return BatchEnqueuedMsg{Err: err}
		}
		if len(rows) == 0 {
			return BatchEnqueuedMsg{PlaylistName: playlistName}
		}
		tracks := playlistRowsToLibTracks(rows)
		ids, err := be.AppendTracks(tracks, playlistName)
		if err != nil {
			return BatchEnqueuedMsg{Err: err}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, playlistName),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: playlistName,
		}
	}, true
}
