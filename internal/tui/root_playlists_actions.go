package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/smartrules"
)

// handlePlaylistKeyAction handles playlist-specific key actions.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handlePlaylistKeyAction(action keys.Action) (Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionPlaylistRename:
		if !m.playlists.AtListLevel() && m.playlists.SmartQuery != nil && m.smartEval != nil {
			q := *m.playlists.SmartQuery
			evaluator := m.smartEval
			pid := m.playlists.PendingLoadID
			return m, func() tea.Msg {
				tracks, err := evaluator.EvalSmartPlaylist(q)
				if err != nil {
					return playlistTracksLoadErrorMsg{err: err}
				}
				return playlistTracksLoadedMsg{tracks: tracks, isRefresh: true, playlistID: pid}
			}, true
		}
		return m, func() tea.Msg { return PlaylistRenameMsg{} }, true
	case keys.ActionPlaylistCreate:
		return m, func() tea.Msg { return PlaylistCreateMsg{} }, true
	case keys.ActionPlaylistOpen:
		return m.handlePlaylistOpen()
	case keys.ActionPlaylistBack:
		if !m.playlists.AtListLevel() {
			m = exitVisualMode(m)
			m.playlists = m.playlists.BackToList()
		}
		return m, nil, true
	case keys.ActionPlaylistEnqueueAll:
		return m.handlePlaylistEnqueueAll()
	case keys.ActionPlaylistDelete:
		m, cmd := m.handlePlaylistDelete()
		return m, cmd, true
	case keys.ActionSmartPlaylistCreate:
		if m.ActiveView == viewPlaylists && m.playlists.AtListLevel() {
			editor := smartrules.NewSmartRuleEditor().WithTags(m.tags)
			m.overlay.smartRuleEditor = &editor
		}
		return m, nil, true
	case keys.ActionMoveTrackUp:
		m, cmd := m.handlePlaylistMoveTrackUp()
		return m, cmd, true
	case keys.ActionMoveTrackDown:
		m, cmd := m.handlePlaylistMoveTrackDown()
		return m, cmd, true
	case keys.ActionSmartPlaylistEdit:
		if m.ActiveView == viewPlaylists && m.playlists.AtListLevel() {
			pl, ok := m.playlists.FocusedPlaylist()
			if ok && pl.IsSmart {
				q, err := db.ParseSmartPlaylistQuery(pl.Query)
				if err != nil {
					m.statusBar.NotifyError("Invalid smart playlist query: " + err.Error())
					return m, nil, true
				}
				editor := smartrules.NewSmartRuleEditorEdit(pl.ID, pl.Name, q).WithTags(m.tags)
				m.overlay.smartRuleEditor = &editor
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
