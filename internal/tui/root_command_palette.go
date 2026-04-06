package tui

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"

	"github.com/sglavoie/muse/internal/config"
)

// commandPaletteActionMsg carries an action to replay through handleKeyAction
// after the palette has been dismissed.
type commandPaletteActionMsg struct {
	action action
}

// configToggleField identifies which audio setting to toggle.
type configToggleField int

const (
	toggleNormalize   configToggleField = iota
	toggleEQ
	toggleStereoWide
)

// configToggleMsg requests that an audio setting be toggled in the config.
type configToggleMsg struct{ field configToggleField }

// randomAlbumMsg switches to Albums view and positions the cursor on a
// randomly chosen album.
type randomAlbumMsg struct{}

// randomArtistMsg switches to Artists view and positions the cursor on a
// randomly chosen artist.
type randomArtistMsg struct{}

// queueShuffleMsg requests an in-place Fisher-Yates shuffle of the queue.
type queueShuffleMsg struct{}

// queueDedupMsg requests removal of duplicate queue entries (keep first
// occurrence of each track hash).
type queueDedupMsg struct{}

// playlistShuffleMsg requests an in-place Fisher-Yates shuffle of the
// focused playlist's tracks.
type playlistShuffleMsg struct{}

// jumpToNowPlayingMsg switches to Library view with the cursor on the
// currently-playing track.
type jumpToNowPlayingMsg struct{}

// buildPaletteCommands returns the list of commands available in the palette.
func (m Model) buildPaletteCommands() []PaletteCommand {
	var cmds []PaletteCommand

	if m.configPath != "" {
		dir := filepath.Dir(m.configPath)
		cmds = append(cmds, PaletteCommand{
			Name: "Open config directory",
			Exec: func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	if m.dataDir != "" {
		dir := m.dataDir
		cmds = append(cmds, PaletteCommand{
			Name: "Open database directory",
			Exec: func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	cmds = append(cmds, PaletteCommand{
		Name: "Show debug panel",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return commandPaletteActionMsg{action: actionDebug} }
		},
	})

	trackFocused := func() bool { _, _, _, found := m.focusedTrackForPalette(); return found }

	// Reveal in Finder / file manager.
	cmds = append(cmds, PaletteCommand{
		Name: "Reveal in Finder",
		Exec: func() tea.Cmd {
			path, _, _, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return revealFile(m.appCtx, path)
		},
		Enabled: trackFocused,
	})

	// Copy track info ("Artist – Title") to clipboard.
	cmds = append(cmds, PaletteCommand{
		Name: "Copy track info",
		Exec: func() tea.Cmd {
			_, artist, title, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			text := fmt.Sprintf("%s \u2013 %s", artist, title)
			return copyToClipboard(m.appCtx, text)
		},
		Enabled: trackFocused,
	})

	// Copy file path to clipboard.
	cmds = append(cmds, PaletteCommand{
		Name: "Copy file path",
		Exec: func() tea.Cmd {
			path, _, _, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return copyToClipboard(m.appCtx, path)
		},
		Enabled: trackFocused,
	})

	// Audio toggles.
	cmds = append(cmds, PaletteCommand{
		Name: "Toggle volume normalization",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return configToggleMsg{field: toggleNormalize} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		Name: "Toggle equalizer",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return configToggleMsg{field: toggleEQ} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		Name: "Toggle stereo widening",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return configToggleMsg{field: toggleStereoWide} }
		},
	})

	// Navigation / discovery.
	cmds = append(cmds, PaletteCommand{
		Name: "Random album",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return randomAlbumMsg{} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		Name: "Random artist",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return randomArtistMsg{} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		Name: "Jump to now-playing in library",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return jumpToNowPlayingMsg{} }
		},
		Enabled: func() bool { return m.playback.currentHash != "" },
	})

	// Queue manipulation.
	queueHasTracks := func() bool { return len(m.queue.entries) > 1 }
	cmds = append(cmds, PaletteCommand{
		Name: "Shuffle queue in place",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return queueShuffleMsg{} }
		},
		Enabled: queueHasTracks,
	})
	cmds = append(cmds, PaletteCommand{
		Name: "Remove queue duplicates",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return queueDedupMsg{} }
		},
		Enabled: queueHasTracks,
	})

	// Playlist manipulation.
	cmds = append(cmds, PaletteCommand{
		Name: "Shuffle playlist in place",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return playlistShuffleMsg{} }
		},
		Enabled: func() bool {
			if m.activeView != viewPlaylists {
				return false
			}
			if m.playlists.atListLevel() {
				pl, ok := m.playlists.focusedPlaylist()
				return ok && !pl.IsSmart && pl.TrackCount > 1
			}
			return m.playlists.smartQuery == nil && len(m.playlists.tracks) > 1
		},
	})

	// Tag filter.
	cmds = append(cmds, PaletteCommand{
		Name: "Filter by tag…",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return openTagFilterPickerMsg{} }
		},
		Enabled: func() bool { return len(m.tags) > 0 },
	})
	if m.activeTagFilter != nil {
		cmds = append(cmds, PaletteCommand{
			Name: "Clear tag filter",
			Exec: func() tea.Cmd {
				return func() tea.Msg { return clearTagFilterMsg{} }
			},
		})
	}

	// Remove all tags from the focused track.
	cmds = append(cmds, PaletteCommand{
		Name: "Remove all tags from track",
		Exec: func() tea.Cmd {
			hash := m.focusedTrackHash()
			if hash == "" {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return func() tea.Msg { return removeAllTagsFromTrackMsg{hash: hash} }
		},
		Enabled: func() bool {
			hash := m.focusedTrackHash()
			return hash != "" && len(m.trackTags[hash]) > 0
		},
	})

	// Per-tag enqueue entries.
	for _, tag := range m.tags {
		t := tag // capture
		cmds = append(cmds, PaletteCommand{
			Name: "Enqueue tag: " + t.Name,
			Exec: func() tea.Cmd {
				return func() tea.Msg { return enqueueByTagMsg{tagID: t.ID, name: t.Name} }
			},
		})
	}

	return cmds
}

// focusedTrackForPalette returns the path, artist, and title of the track
// currently under the cursor, regardless of which view is active. Returns
// found=false when the cursor is not on a track row (e.g., artist/album level).
func (m Model) focusedTrackForPalette() (path, artist, title string, found bool) {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewRecentlyAdded:
		if track, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewHistory:
		if track, ok := m.history.view().FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			return e.Path, e.Artist, e.Title, true
		}

	case viewArtists:
		if m.artists.level == arttui.TrackLevel {
			if track, ok := m.artists.focusedTrack(); ok {
				return track.Path, track.Artist, track.Title, true
			}
		}

	case viewAlbums:
		if m.albums.level == albtui.TrackLevel {
			if track, ok := m.albums.focusedTrack(); ok {
				return track.Path, track.Artist, track.Title, true
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			if m.favorites.category == favtui.CategoryTracks {
				if track, ok := m.favorites.focusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.focusedTrack(); ok {
				return track.Path, track.Artist, track.Title, true
			}
		}

	case viewGenres:
		if m.genres.level == gentui.TrackLevel {
			if track, ok := m.genres.focusedTrack(); ok {
				return track.Path, track.Artist, track.Title, true
			}
		}
	}
	return "", "", "", false
}

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

	m.statusBar.notification = newNotification(note)

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { _ = config.Save(path, cfg); return nil }
}

// onOff returns "On" when b is true, "Off" otherwise.
func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// handleRandomAlbumMsg switches to Albums view and positions the cursor on a
// randomly-chosen album.
func (m Model) handleRandomAlbumMsg() (Model, tea.Cmd) {
	m = m.switchToView(viewAlbums)
	if len(m.albums.albums) == 0 {
		m.statusBar.notification = newNotification("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.albums.albums))
	m.albums.cursor = idx
	m.albums.scrollOffset = max(0, idx-m.albums.height/2)
	m.albums = m.albums.clampCursor()
	return m, nil
}

// handleRandomArtistMsg switches to Artists view and positions the cursor on a
// randomly-chosen artist.
func (m Model) handleRandomArtistMsg() (Model, tea.Cmd) {
	m = m.switchToView(viewArtists)
	if len(m.artists.artists) == 0 {
		m.statusBar.notification = newNotification("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.artists.artists))
	m.artists.cursor = idx
	m.artists.scrollOffset = max(0, idx-m.artists.height/2)
	m.artists = m.artists.clampCursor()
	return m, nil
}

// handleJumpToNowPlayingMsg switches to Library view with the cursor on the
// currently-playing track, clearing any active filter first.
func (m Model) handleJumpToNowPlayingMsg() (Model, tea.Cmd) {
	if m.playback.currentHash == "" {
		m.statusBar.notification = newNotification("Nothing playing")
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
		m.statusBar.notification = newNotification("No playlist focused")
		return m, nil
	}
	if m.playlists.atListLevel() {
		return m.shufflePlaylistFromList()
	}
	return m.shufflePlaylistFromTracks()
}

func (m Model) shufflePlaylistFromTracks() (Model, tea.Cmd) {
	if m.playlists.smartQuery != nil {
		m.statusBar.notification = newNotification("Cannot shuffle smart playlist")
		return m, nil
	}
	if len(m.playlists.tracks) <= 1 {
		m.statusBar.notification = newNotification("Playlist too small to shuffle")
		return m, nil
	}

	tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks))
	copy(tracks, m.playlists.tracks)
	for i := len(tracks) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		tracks[i], tracks[j] = tracks[j], tracks[i]
	}
	m.playlists.tracks = tracks
	m = m.resetPlaylistsSort()

	n := len(tracks)
	m.statusBar.notification = newNotification(fmt.Sprintf("Playlist shuffled (%d tracks)", n))

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
		m.statusBar.notification = newNotification("No playlist focused")
		return m, nil
	}
	if pl.IsSmart {
		m.statusBar.notification = newNotification("Cannot shuffle smart playlist")
		return m, nil
	}
	if pl.TrackCount <= 1 {
		m.statusBar.notification = newNotification("Playlist too small to shuffle")
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
			j := rand.IntN(i + 1)
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

// handleCommandPaletteKeyPress intercepts key presses when the command palette is active.
func (m Model) handleCommandPaletteKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.commandPalette == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.commandPalette.HandleKey(msg.String(), msg)
	m.overlay.commandPalette = &updated
	return m, cmd, true
}
