package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// handlePlaylistKeyAction handles playlist-specific key actions.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handlePlaylistKeyAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionPlaylistRename:
		if !m.playlists.atListLevel() && m.playlists.smartQuery != nil && m.smartEval != nil {
			q := *m.playlists.smartQuery
			evaluator := m.smartEval
			pid := m.playlists.pendingLoadID
			return m, func() tea.Msg {
				tracks, err := evaluator.EvalSmartPlaylist(q)
				if err != nil {
					return playlistTracksLoadErrorMsg{err: err}
				}
				return playlistTracksLoadedMsg{tracks: tracks, isRefresh: true, playlistID: pid}
			}, true
		}
		return m, func() tea.Msg { return PlaylistRenameMsg{} }, true
	case actionPlaylistCreate:
		return m, func() tea.Msg { return PlaylistCreateMsg{} }, true
	case actionPlaylistOpen:
		return m.handlePlaylistOpen()
	case actionPlaylistBack:
		if !m.playlists.atListLevel() {
			m = exitVisualMode(m)
			m.playlists = m.playlists.backToList()
		}
		return m, nil, true
	case actionPlaylistEnqueueAll:
		return m.handlePlaylistEnqueueAll()
	case actionPlaylistDelete:
		m, cmd := m.handlePlaylistDelete()
		return m, cmd, true
	case actionSmartPlaylistCreate:
		if m.activeView == viewPlaylists && m.playlists.atListLevel() {
			editor := newSmartRuleEditor()
			m.overlay.smartRuleEditor = &editor
		}
		return m, nil, true
	case actionMoveTrackUp:
		m, cmd := m.handlePlaylistMoveTrackUp()
		return m, cmd, true
	case actionMoveTrackDown:
		m, cmd := m.handlePlaylistMoveTrackDown()
		return m, cmd, true
	case actionSmartPlaylistEdit:
		if m.activeView == viewPlaylists && m.playlists.atListLevel() {
			pl, ok := m.playlists.focusedPlaylist()
			if ok && pl.IsSmart {
				q, err := db.ParseSmartPlaylistQuery(pl.Query)
				if err != nil {
					m.statusBar.notify("Invalid smart playlist query: " + err.Error())
					return m, nil, true
				}
				editor := newSmartRuleEditorEdit(pl.ID, pl.Name, q)
				m.overlay.smartRuleEditor = &editor
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
