package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleMixOpen drills into the track list for the focused mix.
func (m Model) handleMixOpen() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 0 {
		return m, nil
	}
	mix := m.mixes.focusedMix()
	if mix == nil {
		return m, nil
	}
	m.mixes.openMix(mix.ID)
	return m, m.loadMixCmd(mix.ID)
}

// handleMixBack returns from the track list to the mix list.
func (m Model) handleMixBack() Model {
	if m.activeView != viewMixes {
		return m
	}
	m.mixes.back()
	return m
}

// mixSavedAsPlaylistMsg is returned when a mix is successfully saved as a
// static playlist. It carries the refreshed playlist list, the new playlist
// name, and the number of tracks saved.
type mixSavedAsPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
}

// mixSaveAsPlaylistErrorMsg is returned when saving a mix as a playlist fails.
type mixSaveAsPlaylistErrorMsg struct{ err error }

// handleMixesSaveMixAsPlaylist creates a static playlist from the tracks of
// the currently open mix (level 1 only). It is a no-op at the list level or
// when the mix cache is empty.
func (m Model) handleMixesSaveMixAsPlaylist() (Model, tea.Cmd) {
	if m.mixes.level != 1 {
		return m, nil
	}
	tracks, ok := m.mixes.cachedMixes[m.mixes.activeMix]
	if !ok || len(tracks) == 0 {
		m.statusBar.notify("No tracks to save")
		return m, nil
	}
	var mixName string
	for _, info := range m.mixes.mixes {
		if info.ID == m.mixes.activeMix {
			mixName = info.Name
			break
		}
	}
	name := "Mix: " + mixName
	hashes := make([]string, len(tracks))
	for i, t := range tracks {
		hashes[i] = t.Hash
	}
	count := len(hashes)
	pw := m.playlistWriter
	pa := m.playlistAdder
	if pw == nil {
		m.statusBar.notify("Playlist writer not available")
		return m, nil
	}
	return m, func() tea.Msg {
		id, err := pw.CreatePlaylist(name)
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		if pa != nil && len(hashes) > 0 {
			if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
				return mixSaveAsPlaylistErrorMsg{err: err}
			}
		}
		rows, err := pw.ReadPlaylists()
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		return mixSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count}
	}
}

// handleMixEnqueueAll enqueues all cached tracks from the focused mix (level 0).
func (m Model) handleMixEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 0 {
		return m, nil
	}
	mix := m.mixes.focusedMix()
	if mix == nil {
		return m, nil
	}
	be := m.batchEnqueuer
	if be == nil {
		m.statusBar.notify("Enqueue not available")
		return m, nil
	}
	cached, ok := m.mixes.cachedMixes[mix.ID]
	if !ok || len(cached) == 0 {
		m.statusBar.notify("Load the mix first (press Enter)")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	mixName := mix.Name
	tracks := cached
	return m, func() tea.Msg {
		ids, err := be.AppendTracks(tracks, mixName)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue mix: %w", err)}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, mixName),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: mixName,
		}
	}
}

// handleMixSavedAsPlaylistMsg updates the playlist list and shows a success
// notification after a mix is saved as a static playlist.
func (m Model) handleMixSavedAsPlaylistMsg(msg mixSavedAsPlaylistMsg) Model {
	m.playlists.playlists = msg.playlists
	m = m.preservePlaylistsSort()
	m.statusBar.notify(fmt.Sprintf("Saved as %q (%d %s)", msg.playlistName, msg.count, text.Pluralize(msg.count, "track", "tracks")))
	return m
}
