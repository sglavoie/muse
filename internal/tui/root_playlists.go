package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// --- Message types ---

// playlistTracksLoadedMsg carries the result of a background ReadPlaylistTracks
// call initiated by actionPlaylistOpen. Root handles it by transitioning the
// playlists view to TrackLevel with the loaded rows.
// When isRefresh is true the view is already at TrackLevel and the cursor is
// clamped in-place rather than reset to 0 (smart playlist refresh via 'r').
type playlistTracksLoadedMsg struct {
	tracks    []db.PlaylistTrackRow
	isRefresh bool
}

// playlistTracksLoadErrorMsg carries an error from a failed ReadPlaylistTracks
// call. Root handles it by showing a transient error notification.
type playlistTracksLoadErrorMsg struct{ err error }

// PlaylistRenameMsg is dispatched when the user presses 'r' in the playlists
// view to rename the selected playlist.
type PlaylistRenameMsg struct{}

// playlistRenamedMsg is dispatched after a successful RenamePlaylist call.
// It carries the ID of the renamed playlist so root can reposition the cursor,
// and the refreshed playlist list so root can update the in-memory state.
type playlistRenamedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistRenameErrorMsg is dispatched when RenamePlaylist returns an error.
// Root uses it to cancel the inline rename input and notify the user.
type playlistRenameErrorMsg struct {
	err error
}

// PlaylistCreateMsg is dispatched when the user presses 'c' in the playlists
// view to create a new playlist.
type PlaylistCreateMsg struct{}

// playlistCreatedMsg is dispatched after a successful CreatePlaylist call.
// It carries the refreshed playlist list and the ID of the newly created
// entry so that root can position the cursor on it by ID.
type playlistCreatedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistCreateErrorMsg is dispatched when CreatePlaylist returns an error.
// Root uses it to cancel the inline create input and notify the user.
type playlistCreateErrorMsg struct {
	err error
}

// playlistAddErrorMsg is dispatched when AddTracksToPlaylist returns an error.
// Root uses it to display a transient error notification in the status bar.
type playlistAddErrorMsg struct {
	err error
}

// tracksAddedToPlaylistMsg is dispatched after a successful AddTracksToPlaylist
// call. It carries the refreshed playlist list (for updating track counts) and
// metadata for the status-bar notification.
type tracksAddedToPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
}

// queueSavedAsPlaylistMsg is dispatched after successfully saving the queue as
// a new playlist. It carries the refreshed playlist list and metadata for the
// status-bar notification.
type queueSavedAsPlaylistMsg struct {
	playlists     []db.PlaylistRow
	playlistName  string
	count         int
	fromSelection bool // true when only bulk-selected entries were saved
}

// queueSaveAsPlaylistErrorMsg is dispatched when saving the queue as a
// playlist fails. Root cancels the inline input and shows an error notification.
type queueSaveAsPlaylistErrorMsg struct {
	err error
}

// --- Helper functions ---

// playlistLister is a common interface satisfied by both playlistRenamer
// and playlistCreator for refreshing the playlist list after mutations.
type playlistLister interface {
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// refreshPlaylists reads the full playlist list using whichever lister is
// available. Returns nil rows (not an error) when all are nil.
func refreshPlaylists(listers ...playlistLister) ([]db.PlaylistRow, error) {
	for _, l := range listers {
		if l != nil {
			return l.ReadPlaylists()
		}
	}
	return nil, nil
}

// playlistNameExists checks whether a playlist with the given name already
// exists (case-insensitive), optionally excluding a playlist ID (for rename).
func playlistNameExists(playlists []db.PlaylistRow, name string, excludeID int64) bool {
	for _, p := range playlists {
		if p.ID != excludeID && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// --- Inline input interceptors ---

// handlePlaylistCreateInput intercepts keypresses while the inline playlist
// create input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistCreateInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.isCreating() || m.activeView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.cancelCreate()
		return m, nil, true
	case "enter":
		if !m.playlists.createInputIsEmpty() {
			name := m.playlists.createInputValue()
			if playlistNameExists(m.playlists.playlists, name, 0) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistWriter != nil {
				pw := m.playlistWriter
				return m, func() tea.Msg {
					id, err := pw.CreatePlaylist(name)
					if err != nil {
						return playlistCreateErrorMsg{err: err}
					}
					rows, err := pw.ReadPlaylists()
					if err != nil {
						return playlistCreateErrorMsg{err: err}
					}
					return playlistCreatedMsg{id: id, playlists: rows}
				}, true
			}
			// No writer: dismiss the input without persisting.
			m.playlists = m.playlists.cancelCreate()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateCreateInput(msg)
		return m, cmd, true
	}
}

// handlePlaylistRenameInput intercepts keypresses while the inline playlist
// rename input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.isRenaming() || m.activeView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.cancelRename()
		return m, nil, true
	case "enter":
		if !m.playlists.renameInputIsEmpty() {
			name := m.playlists.renameInputValue()
			pl, ok := m.playlists.focusedPlaylist()
			if !ok {
				m.playlists = m.playlists.cancelRename()
				return m, nil, true
			}
			if playlistNameExists(m.playlists.playlists, name, pl.ID) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistRenamer != nil {
				pr := m.playlistRenamer
				id := pl.ID
				return m, func() tea.Msg {
					if err := pr.RenamePlaylist(id, name); err != nil {
						return playlistRenameErrorMsg{err: err}
					}
					rows, err := pr.ReadPlaylists()
					if err != nil {
						return playlistRenameErrorMsg{err: err}
					}
					return playlistRenamedMsg{id: id, playlists: rows}
				}, true
			}
			// No renamer: dismiss the input without persisting.
			m.playlists = m.playlists.cancelRename()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateRenameInput(msg)
		return m, cmd, true
	}
}

// handleQueueSaveInput intercepts keypresses while the inline save-as-playlist
// input is active. Returns true in the third value when the key was consumed,
// preventing further dispatch.
func (m Model) handleQueueSaveInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.queue.isSavingAsPlaylist() || m.activeView != viewQueue {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.queue = m.queue.cancelSaveAsPlaylist()
		return m, nil, true
	case "enter":
		if !m.queue.saveInputIsEmpty() {
			name := m.queue.saveInputValue()
			if playlistNameExists(m.playlists.playlists, name, 0) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistWriter != nil {
				pw := m.playlistWriter
				pa := m.playlistAdder
				// Capture hashes at dispatch time: selected entries only, or all.
				var hashes []string
				if m.queue.savingFromSelection {
					hashes = m.queue.selectedHashes()
				} else {
					hashes = make([]string, len(m.queue.entries))
					for i, e := range m.queue.entries {
						hashes[i] = e.Hash
					}
				}
				count := len(hashes)
				fromSelection := m.queue.savingFromSelection
				return m, func() tea.Msg {
					id, err := pw.CreatePlaylist(name)
					if err != nil {
						return queueSaveAsPlaylistErrorMsg{err: err}
					}
					if pa != nil && len(hashes) > 0 {
						if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
							return queueSaveAsPlaylistErrorMsg{err: err}
						}
					}
					rows, err := pw.ReadPlaylists()
					if err != nil {
						return queueSaveAsPlaylistErrorMsg{err: err}
					}
					return queueSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count, fromSelection: fromSelection}
				}, true
			}
			// No writer: dismiss the input without persisting.
			m.queue = m.queue.cancelSaveAsPlaylist()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.updateSaveInput(msg)
		return m, cmd, true
	}
}

// --- Message handlers ---

// handlePlaylistTracksLoadedMsg transitions the playlists view to TrackLevel,
// or refreshes the track list in-place when msg.isRefresh is true.
func (m Model) handlePlaylistTracksLoadedMsg(msg playlistTracksLoadedMsg) Model {
	if msg.isRefresh {
		m.playlists = m.playlists.refreshTracks(msg.tracks)
		m.statusBar.notification = newNotification("Smart playlist refreshed")
		return m
	}
	m.playlists = m.playlists.openTracks(msg.tracks)
	return m
}

// handlePlaylistTracksLoadErrorMsg shows a transient error notification for
// a failed playlist track load.
func (m Model) handlePlaylistTracksLoadErrorMsg(msg playlistTracksLoadErrorMsg) Model {
	m.statusBar.notification = newNotification("Load playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaylistRenameMsg starts the inline rename input for the focused playlist.
func (m Model) handlePlaylistRenameMsg() Model {
	if m.playlists.atListLevel() {
		if pl, ok := m.playlists.focusedPlaylist(); ok {
			m.playlists = m.playlists.startRename(pl.Name)
		}
	}
	return m
}

// handlePlaylistRenamedMsg updates the playlist list after a successful rename
// and repositions the cursor on the renamed playlist.
func (m Model) handlePlaylistRenamedMsg(msg playlistRenamedMsg) Model {
	m.playlists.playlists = msg.playlists
	m = m.resetPlaylistsSort()
	m.playlists = m.playlists.cancelRename()
	for i, p := range msg.playlists {
		if p.ID == msg.id {
			m.playlists.cursor = i
			break
		}
	}
	m.playlists = m.playlists.ensureCursorVisible()
	return m
}

// handlePlaylistRenameErrorMsg cancels the inline rename input and shows an
// error notification.
func (m Model) handlePlaylistRenameErrorMsg(msg playlistRenameErrorMsg) Model {
	m.playlists = m.playlists.cancelRename()
	m.statusBar.notification = newNotification("Rename playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaylistCreateMsg starts the inline create input for a new playlist.
func (m Model) handlePlaylistCreateMsg() Model {
	if m.playlists.atListLevel() {
		m.playlists = m.playlists.startCreate()
	}
	return m
}

// handlePlaylistCreatedMsg updates the playlist list after a successful create,
// repositions the cursor on the new playlist, and optionally auto-adds staged
// tracks from the destination picker.
func (m Model) handlePlaylistCreatedMsg(msg playlistCreatedMsg) (Model, tea.Cmd) {
	m.playlists.playlists = msg.playlists
	m = m.resetPlaylistsSort()
	m.playlists = m.playlists.cancelCreate()
	for i, p := range msg.playlists {
		if p.ID == msg.id {
			m.playlists.cursor = i
			break
		}
	}
	m.playlists = m.playlists.ensureCursorVisible()

	// If there are staged tracks from the destination picker, auto-add
	// them to the newly created playlist.
	if len(m.overlay.destPickerTracks) > 0 && m.playlistAdder != nil {
		tracks := m.overlay.destPickerTracks
		m.overlay.destPickerTracks = nil
		pid := msg.id
		pa := m.playlistAdder
		pr := m.playlistRenamer
		pw := m.playlistWriter
		var hashes []string
		for _, t := range tracks {
			hashes = append(hashes, t.Hash)
		}
		// Find the playlist name for the notification.
		var playlistName string
		for _, p := range msg.playlists {
			if p.ID == pid {
				playlistName = p.Name
				break
			}
		}
		count := len(hashes)
		return m, func() tea.Msg {
			if err := pa.AddTracksToPlaylist(pid, hashes); err != nil {
				return playlistAddErrorMsg{err: err}
			}
			rows, err := refreshPlaylists(pr, pw)
			if err != nil {
				slog.Warn("playlist read failed", "err", err)
			}
			return tracksAddedToPlaylistMsg{playlists: rows, playlistName: playlistName, count: count}
		}
	}
	return m, nil
}

// handlePlaylistCreateErrorMsg cancels the inline create input and shows an
// error notification.
func (m Model) handlePlaylistCreateErrorMsg(msg playlistCreateErrorMsg) Model {
	m.playlists = m.playlists.cancelCreate()
	m.statusBar.notification = newNotification("Create playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handleQueueSavedAsPlaylistMsg updates the playlist list after a successful
// save-as-playlist operation and shows a success notification.
func (m Model) handleQueueSavedAsPlaylistMsg(msg queueSavedAsPlaylistMsg) Model {
	m.queue = m.queue.cancelSaveAsPlaylist()
	m.playlists.playlists = msg.playlists
	m = m.resetPlaylistsSort()
	source := "queue"
	if msg.fromSelection {
		source = "selection"
	}
	m.statusBar.notification = newNotification(fmt.Sprintf("Saved %s as %q (%d %s)", source, msg.playlistName, msg.count, pluralize(msg.count, "track", "tracks")))
	player.PlaySound(player.SoundEnqueued)
	return m
}

// handleQueueSaveAsPlaylistErrorMsg cancels the inline save input and shows
// an error notification.
func (m Model) handleQueueSaveAsPlaylistErrorMsg(msg queueSaveAsPlaylistErrorMsg) Model {
	m.queue = m.queue.cancelSaveAsPlaylist()
	m.statusBar.notification = newNotification("Save queue as playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaylistAddErrorMsg shows an error notification for a failed track add.
func (m Model) handlePlaylistAddErrorMsg(msg playlistAddErrorMsg) Model {
	m.statusBar.notification = newNotification("Add tracks failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handleTracksAddedToPlaylistMsg updates the playlist list and shows a success
// notification after tracks are added to a playlist.
func (m Model) handleTracksAddedToPlaylistMsg(msg tracksAddedToPlaylistMsg) Model {
	if msg.playlists != nil {
		m.playlists.playlists = msg.playlists
		m = m.resetPlaylistsSort()
	}
	m.statusBar.notification = newNotification(fmt.Sprintf("%d %s added to %s", msg.count, pluralize(msg.count, "track", "tracks"), msg.playlistName))
	player.PlaySound(player.SoundEnqueued)
	return m
}

// handleDestPickerKeyPress handles key events while the destination picker
// modal is open. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleDestPickerKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.destPicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.destPicker.HandleKey(msg.String())
	m.overlay.destPicker = &updated
	return m, cmd, true
}

// handleDestinationPickerDismissedMsg clears the destination picker and staged tracks.
func (m Model) handleDestinationPickerDismissedMsg() Model {
	m.overlay.destPicker = nil
	m.overlay.destPickerTracks = nil
	m.library = m.library.ClearSelection()
	m.artists.selectedHashes = nil
	m.albums.selectedHashes = nil
	m.genres.selectedHashes = nil
	return m
}

// handleDestinationPickerCreateMsg dismisses the picker, switches to the
// playlists view, and activates the inline create input. Staged tracks are
// kept so they are auto-added after the playlist is created.
func (m Model) handleDestinationPickerCreateMsg() Model {
	m.overlay.destPicker = nil
	m.library = m.library.ClearSelection()
	m.artists.selectedHashes = nil
	m.albums.selectedHashes = nil
	m.genres.selectedHashes = nil
	m.activeView = viewPlaylists
	m.playlists = m.playlists.startCreate()
	return m
}

// handleDestinationPickedMsg processes a destination picker confirmation,
// either enqueueing tracks to the play queue or adding them to a playlist.
func (m Model) handleDestinationPickedMsg(msg pltui.DestinationPickedMsg) (Model, tea.Cmd) {
	tracks := m.overlay.destPickerTracks
	m.overlay.destPicker = nil
	m.overlay.destPickerTracks = nil
	m.library = m.library.ClearSelection()
	m.artists.selectedHashes = nil
	m.albums.selectedHashes = nil
	m.genres.selectedHashes = nil
	if msg.IsQueue {
		// Append tracks to the play queue as a batch so only one sound plays.
		if m.batchEnqueuer != nil {
			be := m.batchEnqueuer
			wasEmpty := len(m.queue.entries) == 0
			return m, func() tea.Msg {
				ids, err := be.AppendTracks(tracks, "")
				if err != nil {
					return BatchEnqueuedMsg{Err: err}
				}
				return BatchEnqueuedMsg{
					Entries:      newQueueEntries(tracks, ids, ""),
					QueueIDs:     ids,
					WasEmpty:     wasEmpty,
					PlaylistName: "Queue",
				}
			}
		}
	} else {
		// Append tracks to the named playlist.
		for _, p := range m.playlists.playlists {
			if p.Name == msg.PlaylistName {
				if m.playlistAdder == nil {
					break
				}
				pid := p.ID
				pa := m.playlistAdder
				pr := m.playlistRenamer
				pw := m.playlistWriter
				var hashes []string
				for _, t := range tracks {
					hashes = append(hashes, t.Hash)
				}
				playlistName := p.Name
				count := len(hashes)
				return m, func() tea.Msg {
					if err := pa.AddTracksToPlaylist(pid, hashes); err != nil {
						return playlistAddErrorMsg{err: err}
					}
					rows, err := refreshPlaylists(pr, pw)
					if err != nil {
						slog.Warn("playlist read failed", "err", err)
					}
					return tracksAddedToPlaylistMsg{playlists: rows, playlistName: playlistName, count: count}
				}
			}
		}
	}
	return m, nil
}

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
		// Find the entry's position in the canonical (unsorted) slice by ID.
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
		m.setUndoSlot(undoDeletePlaylistAction{
			entry: entry,
			idx:   canonicalIdx,
		}, cancel)
		player.PlaySound(player.SoundRemoved)
		m.playlists.playlists = append(m.playlists.playlists[:canonicalIdx], m.playlists.playlists[canonicalIdx+1:]...)
		m = m.resetPlaylistsSort()
		m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
			m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.playlists))
	} else {
		// Smart playlists are read-only — mutations are no-ops.
		if m.playlists.smartQuery != nil {
			return m, nil
		}
		// Bulk remove if tracks are selected.
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
			m.setUndoSlot(undoBulkRemovePlaylistTracksAction{
				entries: removedEntries,
				indices: removedIndices,
			}, cancel)
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
		m.setUndoSlot(undoRemovePlaylistTrackAction{
			entry: entry,
			idx:   idx,
		}, cancel)
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
		// 'r' at TrackLevel of a smart playlist re-evaluates the query.
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
				tracks := make([]library.Track, len(rows))
				for i, r := range rows {
					tracks[i] = library.Track{
						Hash:     r.Hash,
						Path:     r.Path,
						Artist:   r.Artist,
						Title:    r.Title,
						Album:    r.Album,
						Duration: float64(r.DurationMs) / 1000,
					}
				}
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
			tracks := make([]library.Track, len(rows))
			for i, r := range rows {
				tracks[i] = library.Track{
					Hash:     r.Hash,
					Path:     r.Path,
					Artist:   r.Artist,
					Title:    r.Title,
					Album:    r.Album,
					Duration: float64(r.DurationMs) / 1000,
				}
			}
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
	updated, act := m.overlay.smartRuleEditor.HandleKey(msg.String())
	m.overlay.smartRuleEditor = &updated
	switch act {
	case ruleEditorClose:
		m.overlay.smartRuleEditor = nil
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
	return m, nil, true
}

// handleSmartPlaylistCreatedMsg updates the playlist list after a smart playlist
// is created, repositions the cursor on the new entry, and triggers a
// background count evaluation so the list shows actual track counts.
func (m Model) handleSmartPlaylistCreatedMsg(msg smartPlaylistCreatedMsg) (Model, tea.Cmd) {
	m.playlists.playlists = msg.playlists
	m = m.resetPlaylistsSort()
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
	m = m.resetPlaylistsSort()
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
