package tui

import (
	"fmt"
	"math/rand/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

// handleConfigToggleMsg toggles an audio setting, applies it to the player,
// persists the config, and shows a notification.
func (m Model) handleConfigToggleMsg(msg configToggleMsg) (Model, tea.Cmd) {
	var note string
	switch msg.field {
	case toggleNormalize:
		m.cfg.NormalizeVolume = !m.cfg.NormalizeVolume
		if ns, ok := m.player.(audioNormalizer); ok {
			_ = ns.SetNormalize(m.cfg.NormalizeVolume)
		}
		note = "Volume normalization: " + onOff(m.cfg.NormalizeVolume)

	case toggleEQ:
		m.cfg.EQEnabled = !m.cfg.EQEnabled
		if eq, ok := m.player.(equalizerSetter); ok {
			_ = eq.SetEqualizer(m.cfg.EQEnabled, m.cfg.EQBands)
		}
		note = "Equalizer: " + onOff(m.cfg.EQEnabled)

	case toggleStereoWide:
		m.cfg.StereoWideEnabled = !m.cfg.StereoWideEnabled
		if sw, ok := m.player.(stereoWideSetter); ok {
			_ = sw.SetStereoWide(m.cfg.StereoWideEnabled, m.cfg.StereoWideAmount)
		}
		note = "Stereo widening: " + onOff(m.cfg.StereoWideEnabled)
	}

	m.statusBar.notify(note)

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// onOff returns "On" when b is true, "Off" otherwise.
func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// handleRandomAlbumMsg switches to Albums browse mode and positions the cursor
// on a randomly-chosen album.
func (m Model) handleRandomAlbumMsg() (Model, tea.Cmd) {
	m.browse.mode = BrowseModeAlbums
	m = m.switchToView(viewBrowse)
	if len(m.browse.albums.albums) == 0 {
		m.statusBar.notify("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.browse.albums.albums)) //nolint:gosec // non-security random for UI navigation
	m.browse.albums.cursor = idx
	m.browse.albums.scrollOffset = max(0, idx-m.browse.albums.height/2)
	m.browse.albums = m.browse.albums.clampCursor()
	return m, nil
}

// handleRandomArtistMsg switches to Artists browse mode and positions the cursor
// on a randomly-chosen artist.
func (m Model) handleRandomArtistMsg() (Model, tea.Cmd) {
	m.browse.mode = BrowseModeArtists
	m = m.switchToView(viewBrowse)
	if len(m.browse.artists.artists) == 0 {
		m.statusBar.notify("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.browse.artists.artists)) //nolint:gosec // non-security random for UI navigation
	m.browse.artists.cursor = idx
	m.browse.artists.scrollOffset = max(0, idx-m.browse.artists.height/2)
	m.browse.artists = m.browse.artists.clampCursor()
	return m, nil
}

// handleJumpToNowPlayingMsg switches to Library view with the cursor on the
// currently-playing track, clearing any active filter first.
func (m Model) handleJumpToNowPlayingMsg() (Model, tea.Cmd) {
	if m.playback.currentHash == "" {
		m.statusBar.notify("Nothing playing")
		return m, nil
	}
	m.library.filter = m.library.filter.Deactivate()
	m.library.filteredMatches = nil
	m = m.switchToView(viewLibrary)
	pos := m.library.findTrackPosition(m.playback.currentHash)
	m.library.cursor = pos
	m.library = m.library.clampCursor()
	return m, nil
}

// handlePlaylistShuffleMsg applies a Fisher-Yates shuffle to the focused
// playlist's tracks, either in-memory (TrackLevel) or via background load
// (ListLevel), and persists the new order when a playlistShuffler is available.
func (m Model) handlePlaylistShuffleMsg() (Model, tea.Cmd) {
	if m.activeView != viewPlaylists {
		m.statusBar.notify("No playlist focused")
		return m, nil
	}
	if m.playlists.atListLevel() {
		return m.shufflePlaylistFromList()
	}
	return m.shufflePlaylistFromTracks()
}

func (m Model) shufflePlaylistFromTracks() (Model, tea.Cmd) {
	if m.playlists.smartQuery != nil {
		m.statusBar.notify("Cannot shuffle smart playlist")
		return m, nil
	}
	if len(m.playlists.tracks) <= 1 {
		m.statusBar.notify("Playlist too small to shuffle")
		return m, nil
	}

	tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks))
	copy(tracks, m.playlists.tracks)
	for i := len(tracks) - 1; i > 0; i-- {
		j := rand.IntN(i + 1) //nolint:gosec // non-security random for playlist shuffle
		tracks[i], tracks[j] = tracks[j], tracks[i]
	}
	m.playlists.tracks = tracks
	m = m.resetPlaylistsSort()

	n := len(tracks)
	m.statusBar.notify(fmt.Sprintf("Playlist shuffled (%d tracks)", n))

	var playlistID int64
	if m.playlists.listCursor >= 0 && m.playlists.listCursor < len(m.playlists.playlists) {
		playlistID = m.playlists.playlists[m.playlists.listCursor].ID
	}

	if playlistID > 0 {
		if shuffler, ok := m.playlistDeleter.(playlistShuffler); ok {
			ids := make([]int64, n)
			for i, t := range tracks {
				ids[i] = t.ID
			}
			pid := playlistID
			return m, func() tea.Msg { _ = shuffler.SetPlaylistOrder(pid, ids); return nil }
		}
	}

	return m, nil
}

func (m Model) shufflePlaylistFromList() (Model, tea.Cmd) {
	pl, ok := m.playlists.focusedPlaylist()
	if !ok {
		m.statusBar.notify("No playlist focused")
		return m, nil
	}
	if pl.IsSmart {
		m.statusBar.notify("Cannot shuffle smart playlist")
		return m, nil
	}
	if pl.TrackCount <= 1 {
		m.statusBar.notify("Playlist too small to shuffle")
		return m, nil
	}
	if m.playlistReader == nil {
		return m, nil
	}

	reader := m.playlistReader
	var shuffler playlistShuffler
	if s, ok := m.playlistDeleter.(playlistShuffler); ok {
		shuffler = s
	}
	pid := pl.ID
	name := pl.Name

	return m, func() tea.Msg {
		tracks, err := reader.ReadPlaylistTracks(pid)
		if err != nil || len(tracks) <= 1 {
			return TransientNotifyMsg("Playlist too small to shuffle")
		}
		for i := len(tracks) - 1; i > 0; i-- {
			j := rand.IntN(i + 1) //nolint:gosec // non-security random for playlist shuffle
			tracks[i], tracks[j] = tracks[j], tracks[i]
		}
		if shuffler != nil {
			ids := make([]int64, len(tracks))
			for i, t := range tracks {
				ids[i] = t.ID
			}
			_ = shuffler.SetPlaylistOrder(pid, ids)
		}
		return TransientNotifyMsg(fmt.Sprintf("Shuffled %q (%d tracks)", name, len(tracks)))
	}
}
