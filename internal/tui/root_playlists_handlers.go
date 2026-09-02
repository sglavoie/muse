package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handlePlaylistTracksLoadedMsg transitions the playlists view to TrackLevel,
// or refreshes the track list in-place when msg.isRefresh is true.
// The message carries a playlistID so we can discard stale results that arrive
// after the user navigated away (back to list or opened a different playlist).
func (m Model) handlePlaylistTracksLoadedMsg(msg playlistTracksLoadedMsg) Model {
	// Discard stale results: the user navigated away or opened a different playlist.
	if msg.playlistID != 0 && msg.playlistID != m.playlists.pendingLoadID {
		return m
	}
	if msg.isRefresh {
		m.playlists = m.playlists.refreshTracks(msg.tracks)
		m.statusBar.Notify("Smart playlist refreshed")
		return m
	}
	m.playlists = m.playlists.openTracks(msg.tracks)
	return m
}

// handlePlaylistTracksLoadErrorMsg shows a transient error notification for
// a failed playlist track load.
func (m Model) handlePlaylistTracksLoadErrorMsg(msg playlistTracksLoadErrorMsg) Model {
	m.statusBar.Notify("Load playlist failed: " + msg.err.Error())
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
	m = m.preservePlaylistsSort()
	m.playlists = m.playlists.cancelRename()
	for i, p := range m.playlists.effectivePlaylists() {
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
	m.statusBar.Notify("Rename playlist failed: " + msg.err.Error())
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
	m = m.preservePlaylistsSort()
	m.playlists = m.playlists.cancelCreate()
	for i, p := range m.playlists.effectivePlaylists() {
		if p.ID == msg.id {
			m.playlists.cursor = i
			break
		}
	}
	m.playlists = m.playlists.ensureCursorVisible()

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
	m.statusBar.Notify("Create playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handleQueueSavedAsPlaylistMsg updates the playlist list after a successful
// save-as-playlist operation and shows a success notification.
func (m Model) handleQueueSavedAsPlaylistMsg(msg queueSavedAsPlaylistMsg) Model {
	m.queue = m.queue.cancelSaveAsPlaylist()
	m.playlists.playlists = msg.playlists
	m = m.preservePlaylistsSort()
	source := "queue"
	if msg.fromSelection {
		source = "selection"
	}
	m.queueDirty = false
	m.statusBar.Notify(fmt.Sprintf("Saved %s as %q (%d %s)", source, msg.playlistName, msg.count, text.Pluralize(msg.count, "track", "tracks")))
	player.PlaySound(player.SoundEnqueued)
	return m
}

// handleQueueSaveAsPlaylistErrorMsg cancels the inline save input and shows
// an error notification.
func (m Model) handleQueueSaveAsPlaylistErrorMsg(msg queueSaveAsPlaylistErrorMsg) Model {
	m.queue = m.queue.cancelSaveAsPlaylist()
	m.statusBar.Notify("Save queue as playlist failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaylistAddErrorMsg shows an error notification for a failed track add.
func (m Model) handlePlaylistAddErrorMsg(msg playlistAddErrorMsg) Model {
	m.statusBar.Notify("Add tracks failed: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handleTracksAddedToPlaylistMsg updates the playlist list and shows a success
// notification after tracks are added to a playlist.
func (m Model) handleTracksAddedToPlaylistMsg(msg tracksAddedToPlaylistMsg) Model {
	if msg.playlists != nil {
		m.playlists.playlists = msg.playlists
		m = m.preservePlaylistsSort()
	}
	m.statusBar.Notify(fmt.Sprintf("%d %s added to %s", msg.count, text.Pluralize(msg.count, "track", "tracks"), msg.playlistName))
	player.PlaySound(player.SoundEnqueued)
	return m
}

// clearAllSelections clears bulk-selection state across all views that support it.
// Called when the destination picker is dismissed, confirmed, or used to create
// a playlist, so staged selections are always discarded on picker exit.
func (m Model) clearAllSelections() Model {
	m.library = m.library.ClearSelection()
	m.browse.artists.selectedHashes = nil
	m.browse.albums.selectedHashes = nil
	m.browse.genres.selectedHashes = nil
	m.favorites.selectedItems = nil
	m.tagsView.selectedHashes = nil
	m.insights.trackView.selectedHashes = nil
	m.playlists.selectedTrackIDs = nil
	for i := range m.queue.entries {
		m.queue.entries[i].Selected = false
	}
	return m
}

// handleDestinationPickerDismissedMsg clears the destination picker and staged tracks.
func (m Model) handleDestinationPickerDismissedMsg() Model {
	m.overlay.destPicker = nil
	m.overlay.destPickerTracks = nil
	return m.clearAllSelections()
}

// handleDestinationPickerCreateMsg dismisses the picker, switches to the
// playlists view, and activates the inline create input. Staged tracks are
// kept so they are auto-added after the playlist is created. Use
// switchToView to drop visual mode and reset sort state, so the new view
// is not contaminated by the source view's transient state.
func (m Model) handleDestinationPickerCreateMsg() Model {
	m.overlay.destPicker = nil
	m = m.clearAllSelections()
	m = m.switchToView(viewPlaylists)
	m.playlists = m.playlists.startCreate()
	return m
}

// handleDestinationPickedMsg processes a destination picker confirmation,
// either enqueueing tracks to the play queue or adding them to a playlist.
func (m Model) handleDestinationPickedMsg(msg pltui.DestinationPickedMsg) (Model, tea.Cmd) {
	tracks := m.overlay.destPickerTracks
	m.overlay.destPicker = nil
	m.overlay.destPickerTracks = nil
	m = m.clearAllSelections()
	if msg.IsQueue {
		if cmd := m.batchEnqueueTracksCmd(tracks, "", "Queue"); cmd != nil {
			return m, cmd
		}
	} else {
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
