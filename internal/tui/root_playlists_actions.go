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

// handlePlaylistOpen opens a playlist (loading tracks) or enqueues the focused
// track when already inside a playlist at track level.
func (m Model) handlePlaylistOpen() (Model, tea.Cmd, bool) {
	if !m.playlists.atListLevel() {
		if m.queueWriter != nil {
			if cmd := m.playlists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
		return m, nil, true
	}

	pl, ok := m.playlists.focusedPlaylist()
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
	m.playlists.smartQuery = &q
	m.playlists.pendingLoadID = pl.ID
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
	m.playlists.pendingLoadID = pid
	reader := m.playlistReader
	return m, func() tea.Msg {
		tracks, err := reader.ReadPlaylistTracks(pid)
		if err != nil {
			return playlistTracksLoadErrorMsg{err: err}
		}
		return playlistTracksLoadedMsg{tracks: tracks, playlistID: pid}
	}, true
}

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
	if m.playlists.atListLevel() {
		pl, found := m.playlists.focusedPlaylist()
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

	if m.playlists.listCursor < 0 || m.playlists.listCursor >= len(m.playlists.playlists) {
		return 0, "", false, nil, false
	}
	entry := m.playlists.playlists[m.playlists.listCursor]
	return entry.ID, entry.Name, m.playlists.smartQuery != nil, m.playlists.smartQuery, true
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
