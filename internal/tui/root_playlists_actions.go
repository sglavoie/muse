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
		m = m.resetPlaylistsSort()
		m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
			m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.playlists))
	} else {
		if m.playlists.smartQuery != nil {
			return m, nil
		}
		if m.playlists.hasTrackSelection() {
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
			m = m.resetPlaylistsSort()
			m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
				m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
			m.playlists = m.playlists.clearTrackSelection()
			m.setUndoSlot(undoBulkRemovePlaylistTracksAction{entries: removedEntries, indices: removedIndices}, cancel)
			player.PlaySound(player.SoundRemoved)
			return m, nil
		}
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
		m = m.resetPlaylistsSort()
		m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
			m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
	}
	return m, nil
}

// handlePlaylistKeyAction handles playlist-specific key actions.
// Returns (model, cmd, true) when the action was consumed.
func (m Model) handlePlaylistKeyAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionPlaylistRename:
		if !m.playlists.atListLevel() && m.playlists.smartQuery != nil && m.smartEval != nil {
			q := *m.playlists.smartQuery
			evaluator := m.smartEval
			return m, func() tea.Msg {
				tracks, err := evaluator.EvalSmartPlaylist(q)
				if err != nil {
					return playlistTracksLoadErrorMsg{err: err}
				}
				return playlistTracksLoadedMsg{tracks: tracks, isRefresh: true}
			}, true
		}
		return m, func() tea.Msg { return PlaylistRenameMsg{} }, true
	case actionPlaylistCreate:
		return m, func() tea.Msg { return PlaylistCreateMsg{} }, true
	case actionPlaylistOpen:
		if m.playlists.atListLevel() {
			pl, ok := m.playlists.focusedPlaylist()
			if ok {
				if pl.IsSmart && m.smartEval != nil {
					q, err := db.ParseSmartPlaylistQuery(pl.Query)
					if err != nil {
						return m, func() tea.Msg { return playlistTracksLoadErrorMsg{err: err} }, true
					}
					m.playlists.smartQuery = &q
					evaluator := m.smartEval
					return m, func() tea.Msg {
						tracks, err := evaluator.EvalSmartPlaylist(q)
						if err != nil {
							return playlistTracksLoadErrorMsg{err: err}
						}
						return playlistTracksLoadedMsg{tracks: tracks}
					}, true
				} else if !pl.IsSmart && m.playlistReader != nil {
					pid := pl.ID
					reader := m.playlistReader
					return m, func() tea.Msg {
						tracks, err := reader.ReadPlaylistTracks(pid)
						if err != nil {
							return playlistTracksLoadErrorMsg{err: err}
						}
						return playlistTracksLoadedMsg{tracks: tracks}
					}, true
				}
			}
		} else if !m.playlists.atListLevel() && m.queueWriter != nil {
			if cmd := m.playlists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
		return m, nil, true
	case actionPlaylistBack:
		if !m.playlists.atListLevel() {
			m.playlists = m.playlists.backToList()
		}
		return m, nil, true
	case actionPlaylistEnqueueAll:
		if m.batchEnqueuer == nil {
			return m, nil, true
		}
		var isSmart bool
		var smartQuery *db.SmartPlaylistQuery
		var pid int64
		var playlistName string
		if m.playlists.atListLevel() {
			pl, ok := m.playlists.focusedPlaylist()
			if !ok {
				return m, nil, true
			}
			pid = pl.ID
			playlistName = pl.Name
			isSmart = pl.IsSmart
			if isSmart {
				q, err := db.ParseSmartPlaylistQuery(pl.Query)
				if err != nil || m.smartEval == nil {
					return m, nil, true
				}
				smartQuery = &q
			}
		} else {
			if m.playlists.listCursor < 0 || m.playlists.listCursor >= len(m.playlists.playlists) {
				return m, nil, true
			}
			pid = m.playlists.playlists[m.playlists.listCursor].ID
			playlistName = m.playlists.playlists[m.playlists.listCursor].Name
			isSmart = m.playlists.smartQuery != nil
			smartQuery = m.playlists.smartQuery
		}
		if isSmart && smartQuery != nil {
			evaluator := m.smartEval
			q := *smartQuery
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
				tracks := playlistTracksToLibraryTracks(rows)
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
		if m.playlistReader == nil {
			return m, nil, true
		}
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
			tracks := playlistTracksToLibraryTracks(rows)
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
	case actionPlaylistDelete:
		m, cmd := m.handlePlaylistDelete()
		return m, cmd, true
	case actionSmartPlaylistCreate:
		if m.activeView == viewPlaylists && m.playlists.atListLevel() {
			editor := newSmartRuleEditor()
			m.overlay.smartRuleEditor = &editor
		}
		return m, nil, true
	case actionSmartPlaylistEdit:
		if m.activeView == viewPlaylists && m.playlists.atListLevel() {
			pl, ok := m.playlists.focusedPlaylist()
			if ok && pl.IsSmart {
				q, err := db.ParseSmartPlaylistQuery(pl.Query)
				if err != nil {
					m.statusBar.notification = newNotification("Invalid smart playlist query: " + err.Error())
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

// handleSmartRuleEditorKeyPress routes key events to the smart rule editor
// overlay when it is active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleSmartRuleEditorKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.smartRuleEditor == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, act, cmd := m.overlay.smartRuleEditor.HandleKey(msg)
	m.overlay.smartRuleEditor = &updated
	switch act {
	case ruleEditorClose:
		m.overlay.smartRuleEditor = nil
		return m, cmd, true
	case ruleEditorCommit:
		name := updated.Name()
		q := updated.buildQuery()
		isEdit := updated.isEdit
		editID := updated.editID
		m.overlay.smartRuleEditor = nil
		sw := m.smartPlaylistWriter
		if sw != nil {
			return m, func() tea.Msg {
				queryJSON, err := db.MarshalSmartPlaylistQuery(q)
				if err != nil {
					return TransientNotifyMsg("Smart playlist error: " + err.Error())
				}
				if isEdit {
					if err := sw.UpdateSmartPlaylistQuery(editID, name, queryJSON); err != nil {
						return TransientNotifyMsg("Update failed: " + err.Error())
					}
					rows, err := sw.ReadPlaylists()
					if err != nil {
						return TransientNotifyMsg("Refresh failed: " + err.Error())
					}
					return smartPlaylistEditedMsg{playlists: rows}
				}
				id, err := sw.CreateSmartPlaylist(name, queryJSON)
				if err != nil {
					return TransientNotifyMsg("Create failed: " + err.Error())
				}
				rows, err := sw.ReadPlaylists()
				if err != nil {
					return TransientNotifyMsg("Refresh failed: " + err.Error())
				}
				return smartPlaylistCreatedMsg{id: id, playlists: rows}
			}, true
		}
	}
	return m, cmd, true
}
