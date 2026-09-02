package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// handlePlaylistOpen opens a playlist (loading tracks) or enqueues the focused
// track when already inside a playlist at track level.
func (m Model) handlePlaylistOpen() (Model, tea.Cmd, bool) {
	if !m.playlists.AtListLevel() {
		if m.queueWriter != nil {
			if cmd := m.playlists.EnqueueFocusedTrack(m.queueWriter, len(m.queue.Entries)); cmd != nil {
				return m, cmd, true
			}
		}
		return m, nil, true
	}

	pl, ok := m.playlists.FocusedPlaylist()
	if !ok {
		return m, nil, true
	}

	if pl.IsSmart && m.smartEval != nil {
		return m.openSmartPlaylist(pl)
	}
	if !pl.IsSmart && m.playlistReader != nil {
		return m.openRegularPlaylist(pl)
	}
	return m, nil, true
}

func (m Model) openSmartPlaylist(pl db.PlaylistRow) (Model, tea.Cmd, bool) {
	q, err := db.ParseSmartPlaylistQuery(pl.Query)
	if err != nil {
		return m, func() tea.Msg { return playlistTracksLoadErrorMsg{err: err} }, true
	}
	m.playlists.SmartQuery = &q
	m.playlists.PendingLoadID = pl.ID
	evaluator := m.smartEval
	pid := pl.ID
	return m, func() tea.Msg {
		tracks, err := evaluator.EvalSmartPlaylist(q)
		if err != nil {
			return playlistTracksLoadErrorMsg{err: err}
		}
		return playlistTracksLoadedMsg{tracks: tracks, playlistID: pid}
	}, true
}

func (m Model) openRegularPlaylist(pl db.PlaylistRow) (Model, tea.Cmd, bool) {
	pid := pl.ID
	m.playlists.PendingLoadID = pid
	reader := m.playlistReader
	return m, func() tea.Msg {
		tracks, err := reader.ReadPlaylistTracks(pid)
		if err != nil {
			return playlistTracksLoadErrorMsg{err: err}
		}
		return playlistTracksLoadedMsg{tracks: tracks, playlistID: pid}
	}, true
}
