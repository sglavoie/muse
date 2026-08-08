package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// buildPaletteCommands returns the list of commands available in the palette.
func (m Model) buildPaletteCommands() []PaletteCommand {
	var cmds []PaletteCommand

	// Pinned anchors (Serial Position Effect): Help at top, Quit at bottom.
	cmds = append(cmds, paletteActionPin("Help", "?", actionHelp, pinTop))

	if m.configPath != "" {
		dir := filepath.Dir(m.configPath)
		cmds = append(cmds, PaletteCommand{
			ID:       "dir.open.config",
			Name:     "Open config directory",
			Category: "file",
			Exec:     func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	if m.dataDir != "" {
		dir := m.dataDir
		cmds = append(cmds, PaletteCommand{
			ID:       "dir.open.data",
			Name:     "Open database directory",
			Category: "file",
			Exec:     func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	cmds = append(cmds, paletteActionCmd("dev", "Show debug panel", "ctrl+g", actionDebug))
	cmds = append(cmds, paletteActionCmd("mode", "Toggle radio mode (Auto-DJ)", "A", actionRadioToggle))

	// Stable navigation targets. Library and Browse use explicit mode messages
	// because their numeric shortcuts preserve the current sub-mode; palette
	// commands should always land on the destination named in the result.
	cmds = append(cmds,
		paletteMessageCmd("nav.library.all", "nav", "Go to Library: All Tracks", "", navigateLibraryModeMsg{mode: LibraryModeAll}),
		paletteMessageCmd("nav.library.recent", "nav", "Go to Library: Recently Added", "", navigateLibraryModeMsg{mode: LibraryModeRecent}),
		paletteMessageCmd("nav.library.history", "nav", "Go to Library: History", "", navigateLibraryModeMsg{mode: LibraryModeHistory}),
		paletteActionCmd("nav", "Go to Playlists", "2", actionSwitchPlaylists),
		paletteActionCmd("nav", "Go to Queue", "3", actionSwitchQueue),
		paletteMessageCmd("nav.browse.artists", "nav", "Go to Browse: Artists", "", navigateBrowseModeMsg{mode: BrowseModeArtists}),
		paletteMessageCmd("nav.browse.albums", "nav", "Go to Browse: Albums", "", navigateBrowseModeMsg{mode: BrowseModeAlbums}),
		paletteMessageCmd("nav.browse.genres", "nav", "Go to Browse: Genres", "", navigateBrowseModeMsg{mode: BrowseModeGenres}),
		paletteActionCmd("nav", "Go to Favorites", "5", actionSwitchFavorites),
		paletteActionCmd("nav", "Go to Tags", "6", actionSwitchTags),
		paletteActionCmd("nav", "Go to Mixes", "7", actionSwitchMixes),
	)

	cmds = append(cmds, PaletteCommand{
		ID:       "mixes.refresh",
		Name:     "Refresh mixes",
		Category: "lib",
		Exec:     m.loadMixCountsCmd,
		Enabled:  func() bool { return m.recReader != nil },
	})

	trackFocused := func() bool { _, _, _, found := m.focusedTrackForPalette(); return found }

	cmds = append(cmds, PaletteCommand{
		ID:       "track.reveal",
		Name:     "Reveal in Finder",
		Category: "file",
		Exec: func() tea.Cmd {
			path, _, _, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return revealFile(m.appCtx, path)
		},
		Enabled: trackFocused,
	})

	cmds = append(cmds, PaletteCommand{
		ID:       "track.copy.info",
		Name:     "Copy track info",
		Category: "file",
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

	cmds = append(cmds, PaletteCommand{
		ID:       "track.copy.path",
		Name:     "Copy file path",
		Category: "file",
		Exec: func() tea.Cmd {
			path, _, _, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return copyToClipboard(m.appCtx, path)
		},
		Enabled: trackFocused,
	})

	cmds = append(cmds, PaletteCommand{
		ID:       "ui.toggle.footer_hints",
		Name:     "Toggle footer hints",
		Category: "mode",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return toggleFooterHintsMsg{} }
		},
	})

	cmds = append(cmds, PaletteCommand{
		ID:       "queue.toggle.follow",
		Name:     "Toggle queue follow",
		Category: "mode",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return toggleQueueFollowMsg{} }
		},
	})

	cmds = append(cmds, configTogglePalette("audio.toggle.normalize", "mode", "Toggle volume normalization", toggleNormalize))
	cmds = append(cmds, configTogglePalette("audio.toggle.eq", "mode", "Toggle equalizer", toggleEQ))
	cmds = append(cmds, configTogglePalette("audio.toggle.stereo", "mode", "Toggle stereo widening", toggleStereoWide))

	cmds = append(cmds, PaletteCommand{
		ID:       "random.album",
		Name:     "Random album",
		Category: "play",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return randomAlbumMsg{} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		ID:       "random.artist",
		Name:     "Random artist",
		Category: "play",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return randomArtistMsg{} }
		},
	})
	cmds = append(cmds, PaletteCommand{
		ID:       "library.jump.nowplaying",
		Name:     "Jump to now-playing in library",
		Category: "nav",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return jumpToNowPlayingMsg{} }
		},
		Enabled: func() bool { return m.playback.currentHash != "" },
	})

	queueHasTracks := func() bool { return len(m.queue.entries) > 1 }
	cmds = append(cmds, PaletteCommand{
		ID:       "queue.shuffle",
		Name:     "Shuffle queue in place",
		Category: "play",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return queueShuffleMsg{} }
		},
		Enabled: queueHasTracks,
	})
	cmds = append(cmds, PaletteCommand{
		ID:       "queue.dedup",
		Name:     "Remove queue duplicates",
		Category: "play",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return queueDedupMsg{} }
		},
		Enabled: queueHasTracks,
	})

	cmds = append(cmds, PaletteCommand{
		ID:       "playlist.shuffle",
		Name:     "Shuffle playlist in place",
		Category: "play",
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

	cmds = append(cmds, PaletteCommand{
		ID:       "tag.filter.open",
		Name:     "Filter by tag…",
		Category: "tag",
		Shortcut: "ctrl+t",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return openTagFilterPickerMsg{} }
		},
		Enabled: func() bool { return len(nonEmptyTags(m.tags, m.tagsView.tagCounts)) > 0 },
	})
	if m.activeTagFilter != nil {
		cmds = append(cmds, PaletteCommand{
			ID:       "tag.filter.clear",
			Name:     "Clear tag filter",
			Category: "tag",
			Shortcut: "ctrl+t",
			Exec: func() tea.Cmd {
				return func() tea.Msg { return clearTagFilterMsg{} }
			},
		})
	}

	cmds = append(cmds, PaletteCommand{
		ID:       "tag.track.remove_all",
		Name:     "Remove all tags from track",
		Category: "tag",
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

	for _, tag := range m.tags {
		if m.tagsView.tagCounts != nil && m.tagsView.tagCounts[tag.ID] == 0 {
			continue
		}
		t := tag
		cmds = append(cmds, PaletteCommand{
			ID:       fmt.Sprintf("tag.enqueue.%d", t.ID),
			Name:     "Enqueue tag: " + t.Name,
			Category: "tag",
			Exec: func() tea.Cmd {
				return func() tea.Msg { return enqueueByTagMsg{tagID: t.ID, name: t.Name} }
			},
		})
	}

	cmds = append(cmds, paletteActionPin("Quit", "ctrl+c", actionQuit, pinBottom))

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

	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			return e.Path, e.Artist, e.Title, true
		}

	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.level == arttui.TrackLevel {
				if track, ok := m.browse.artists.focusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case BrowseModeAlbums:
			if m.browse.albums.level == albtui.TrackLevel {
				if track, ok := m.browse.albums.focusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case BrowseModeGenres:
			if m.browse.genres.level == gentui.TrackLevel {
				if track, ok := m.browse.genres.focusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
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

	case viewTags:
		if track, ok := m.tagsView.focusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}
	case viewMixes:
		if t := m.mixes.focusedTrack(); t != nil {
			return t.Path, t.Artist, t.Title, true
		}
	}

	return "", "", "", false
}

// paletteActionCmd returns a PaletteCommand that dispatches the given action
// through the command palette action pipeline. The ID is derived from the
// action enum value for stable usage tracking.
func paletteActionCmd(category, name, shortcut string, a action) PaletteCommand {
	return PaletteCommand{
		ID:       fmt.Sprintf("action.%d", a),
		Name:     name,
		Category: category,
		Shortcut: shortcut,
		Exec:     func() tea.Cmd { return func() tea.Msg { return commandPaletteActionMsg{action: a} } },
	}
}

// paletteActionPin returns a PaletteCommand that dispatches the given action
// and is anchored at the top or bottom of the palette result list.
func paletteActionPin(name, shortcut string, a action, pin PalettePin) PaletteCommand {
	c := paletteActionCmd("", name, shortcut, a)
	c.Pinned = pin
	return c
}

// paletteMessageCmd returns a command with a stable explicit identifier that
// dispatches an internal message through the normal root update pipeline.
func paletteMessageCmd(id, category, name, shortcut string, msg tea.Msg) PaletteCommand {
	return PaletteCommand{
		ID:       id,
		Name:     name,
		Category: category,
		Shortcut: shortcut,
		Exec:     func() tea.Cmd { return func() tea.Msg { return msg } },
	}
}

// configTogglePalette returns a PaletteCommand that sends a configToggleMsg
// for the given audio setting field.
func configTogglePalette(id, category, name string, field configToggleField) PaletteCommand {
	return PaletteCommand{
		ID:       id,
		Name:     name,
		Category: category,
		Exec:     func() tea.Cmd { return func() tea.Msg { return configToggleMsg{field: field} } },
	}
}
