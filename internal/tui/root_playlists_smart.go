package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// handleSmartPlaylistCreatedMsg updates the playlist list after a smart playlist
// is created, repositions the cursor on the new entry, and triggers a
// background count evaluation so the list shows actual track counts.
func (m Model) handleSmartPlaylistCreatedMsg(msg smartPlaylistCreatedMsg) (Model, tea.Cmd) {
	m.playlists.playlists = msg.playlists
	m = m.preservePlaylistsSort()
	for i, p := range msg.playlists {
		if p.ID == msg.id {
			m.playlists.cursor = i
			break
		}
	}
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Smart playlist created")
	return m, evalSmartPlaylistCountsCmd(msg.playlists, m.smartEval)
}

// handleSmartPlaylistEditedMsg updates the playlist list after a smart playlist
// query is edited and triggers a background count evaluation.
func (m Model) handleSmartPlaylistEditedMsg(msg smartPlaylistEditedMsg) (Model, tea.Cmd) {
	m.playlists.playlists = msg.playlists
	m = m.preservePlaylistsSort()
	m.playlists = m.playlists.ensureCursorVisible()
	m.statusBar.notification = newNotification("Smart playlist updated")
	return m, evalSmartPlaylistCountsCmd(msg.playlists, m.smartEval)
}

// handleSmartPlaylistCountsUpdatedMsg patches TrackCount for smart playlists
// whose counts have been evaluated in the background.
func (m Model) handleSmartPlaylistCountsUpdatedMsg(msg smartPlaylistCountsUpdatedMsg) Model {
	for i, p := range m.playlists.playlists {
		if p.IsSmart {
			if count, ok := msg.counts[p.ID]; ok {
				m.playlists.playlists[i].TrackCount = count
			}
		}
	}
	return m
}

// evalSmartPlaylistCountsCmd launches a background goroutine that evaluates
// every smart playlist in the list and returns a smartPlaylistCountsUpdatedMsg.
// Returns nil when there are no smart playlists or the evaluator is nil.
func evalSmartPlaylistCountsCmd(playlists []db.PlaylistRow, evaluator smartPlaylistEvaluator) tea.Cmd {
	if evaluator == nil {
		return nil
	}
	var smart []db.PlaylistRow
	for _, p := range playlists {
		if p.IsSmart {
			smart = append(smart, p)
		}
	}
	if len(smart) == 0 {
		return nil
	}
	return func() tea.Msg {
		counts := make(map[int64]int, len(smart))
		for _, p := range smart {
			q, err := db.ParseSmartPlaylistQuery(p.Query)
			if err != nil {
				counts[p.ID] = 0
				continue
			}
			tracks, err := evaluator.EvalSmartPlaylist(q)
			if err != nil {
				counts[p.ID] = 0
				continue
			}
			counts[p.ID] = len(tracks)
		}
		return smartPlaylistCountsUpdatedMsg{counts: counts}
	}
}
