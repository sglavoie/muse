package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleMixOpen drills into the track list for the focused mix.
func (m Model) handleMixOpen() (Model, tea.Cmd) {
	if m.ActiveView == viewMixes && m.mixes.Level == 1 {
		next, cmd, _ := m.handleLibraryEnqueueAction()
		return next.(Model), cmd
	}
	if m.ActiveView != viewMixes || m.mixes.Level != 0 {
		return m, nil
	}
	mix := m.mixes.FocusedMix()
	if mix == nil {
		return m, nil
	}
	m.mixes.OpenMix(mix.ID)
	return m, m.loadMixCmd(mix.ID)
}

// handleMixBack returns from the track list to the mix list.
func (m Model) handleMixBack() Model {
	if m.ActiveView != viewMixes {
		return m
	}
	if m.visual.active {
		return exitVisualMode(m)
	}
	m.mixes.Back()
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
	if m.mixes.Level != 1 {
		return m, nil
	}
	tracks, ok := m.mixes.CachedMixes[m.mixes.ActiveMix]
	if !ok || len(tracks) == 0 {
		m.statusBar.Notify("No tracks to save")
		return m, nil
	}
	var mixName string
	for _, info := range m.mixes.Mixes {
		if info.ID == m.mixes.ActiveMix {
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
		m.statusBar.Notify("Playlist writer not available")
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

// handleMixEnqueueAll enqueues all cached tracks from the focused mix at the
// list level (0), or from the open mix at the track level (1).
func (m Model) handleMixEnqueueAll() (Model, tea.Cmd) {
	if m.ActiveView != viewMixes {
		return m, nil
	}
	mix := m.mixes.FocusedMix()
	if m.mixes.Level == 1 {
		mix = m.openedMix()
	}
	if mix == nil {
		return m, nil
	}
	be := m.batchEnqueuer
	if be == nil {
		m.statusBar.Notify("Enqueue not available")
		return m, nil
	}
	cached, ok := m.mixes.CachedMixes[mix.ID]
	if !ok || len(cached) == 0 {
		if m.mixes.Level == 1 {
			m.statusBar.Notify("No tracks to enqueue")
		} else {
			m.statusBar.Notify("Load the mix first (press enter)")
		}
		return m, nil
	}
	wasEmpty := len(m.queue.Entries) == 0
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

// openedMix returns the mix whose tracks are open at level 1, or nil when no
// mix is open or it is no longer listed.
func (m Model) openedMix() *recommendation.MixInfo {
	if m.mixes.Level != 1 {
		return nil
	}
	for i := range m.mixes.Mixes {
		if m.mixes.Mixes[i].ID == m.mixes.ActiveMix {
			return &m.mixes.Mixes[i]
		}
	}
	return nil
}

// handleMixSavedAsPlaylistMsg updates the playlist list and shows a success
// notification after a mix is saved as a static playlist.
func (m Model) handleMixSavedAsPlaylistMsg(msg mixSavedAsPlaylistMsg) Model {
	m.playlists.Playlists = msg.playlists
	m = m.preservePlaylistsSort()
	m.statusBar.Notify(fmt.Sprintf("Saved as %q (%d %s)", msg.playlistName, msg.count, text.Pluralize(msg.count, "track", "tracks")))
	return m
}
