package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/desktop"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// buildPaletteCommands returns the list of commands available in the palette.
func (m Model) buildPaletteCommands() []PaletteCommand {
	var cmds []PaletteCommand

	for _, entry := range paletteCatalog {
		cmds = append(cmds, m.paletteCommandFor(entry))
	}

	if m.configPath != "" {
		dir := filepath.Dir(m.configPath)
		cmds = append(cmds, PaletteCommand{
			ID:       "dir.open.config",
			Name:     "Open config directory",
			Category: "file",
			Exec:     func() tea.Cmd { return desktop.OpenDir(m.appCtx, dir) },
		})
	}

	if m.dataDir != "" {
		dir := m.dataDir
		cmds = append(cmds, PaletteCommand{
			ID:       "dir.open.data",
			Name:     "Open database directory",
			Category: "file",
			Exec:     func() tea.Cmd { return desktop.OpenDir(m.appCtx, dir) },
		})
	}

	// Hand-written rather than catalog entries: the palette is the only trigger
	// (no keybinding), so neither needs an action constant or a registry binding.
	cmds = append(cmds, PaletteCommand{
		ID:       "metadata.export.tsv",
		Name:     "Export visible tracks to TSV",
		Category: "lib",
		Exec:     func() tea.Cmd { return func() tea.Msg { return metadataExportRequestedMsg{} } },
		Enabled:  func() bool { return m.dataDir != "" },
	})
	cmds = append(cmds, PaletteCommand{
		ID:       "metadata.import",
		Name:     "Import metadata edits…",
		Category: "lib",
		Exec:     func() tea.Cmd { return func() tea.Msg { return metadataImportRequestedMsg{} } },
		Enabled:  func() bool { return metadataExportExists(m.dataDir) },
	})
	cmds = append(cmds, PaletteCommand{
		ID:       "metadata.export.reveal",
		Name:     "Reveal metadata export file",
		Category: "lib",
		Exec:     func() tea.Cmd { return desktop.RevealFile(m.appCtx, metadataExportPath(m.dataDir)) },
		Enabled:  func() bool { return metadataExportExists(m.dataDir) },
	})

	// Stable navigation targets. Library and Browse use explicit mode messages
	// because their numeric shortcuts preserve the current sub-mode; palette
	// commands should always land on the destination named in the result.
	cmds = append(cmds,
		paletteMessageCmd("nav.library.all", "nav", "Go to Library: All Tracks", "", navigateLibraryModeMsg{mode: LibraryModeAll}),
		paletteMessageCmd("nav.library.recent", "nav", "Go to Library: Recently Added", "", navigateLibraryModeMsg{mode: LibraryModeRecent}),
		paletteMessageCmd("nav.library.history", "nav", "Go to Library: History", "", navigateLibraryModeMsg{mode: LibraryModeHistory}),
		paletteMessageCmd("nav.browse.artists", "nav", "Go to Browse: Artists", "", navigateBrowseModeMsg{mode: BrowseModeArtists}),
		paletteMessageCmd("nav.browse.albums", "nav", "Go to Browse: Albums", "", navigateBrowseModeMsg{mode: BrowseModeAlbums}),
		paletteMessageCmd("nav.browse.genres", "nav", "Go to Browse: Genres", "", navigateBrowseModeMsg{mode: BrowseModeGenres}),
	)

	cmds = append(cmds, PaletteCommand{
		ID:       "mixes.refresh",
		Name:     "Refresh mixes",
		Category: "lib",
		Exec:     m.loadMixCountsCmd,
		Enabled:  func() bool { return m.recReader != nil },
	})

	trackFocused := func() bool { return hasFocusedTrack(m) }

	cmds = append(cmds, PaletteCommand{
		ID:       "track.reveal",
		Name:     "Reveal in Finder",
		Category: "file",
		Exec: func() tea.Cmd {
			path, _, _, found := m.focusedTrackForPalette()
			if !found {
				return func() tea.Msg { return TransientNotifyMsg("No track focused") }
			}
			return desktop.RevealFile(m.appCtx, path)
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
			return desktop.CopyToClipboard(m.appCtx, text)
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
			return desktop.CopyToClipboard(m.appCtx, path)
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
		ID:       "ui.toggle.track_notify",
		Name:     "Toggle track notifications",
		Category: "mode",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return toggleTrackNotifyMsg{} }
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
	case viewInsights:
		if t := m.insights.focusedTrack(); t != nil {
			return t.Path, t.Artist, t.Title, true
		}
	}

	return "", "", "", false
}

// paletteCommandFor builds the palette command for a catalog entry. The
// action is dispatched through the command palette action pipeline and the
// shortcut is derived from the live registry, so a rebind cannot leave a stale
// hint behind.
func (m Model) paletteCommandFor(e paletteEntry) PaletteCommand {
	cmd := PaletteCommand{
		ID:       e.id,
		Name:     e.name,
		Category: e.category,
		Shortcut: m.paletteShortcutFor(e),
		Pinned:   e.pin,
		Exec:     func() tea.Cmd { return func() tea.Msg { return commandPaletteActionMsg{action: e.act, view: e.view} } },
	}
	if e.enabled != nil {
		// e.enabled is evaluated here, against the pre-switch model, because
		// the row is rendered before the switch happens.
		cmd.Enabled = func() bool { return e.enabled(m) }
	}
	return cmd
}

// paletteShortcutFor returns the key to advertise for a catalog entry:
// preferKey when set, otherwise the first key bound to the action in the
// entry's own context, falling back to contextGlobal.
func (m Model) paletteShortcutFor(e paletteEntry) string {
	if e.preferKey != "" {
		return e.preferKey
	}
	if bound := m.registry.KeysFor(e.ctx, e.act); len(bound) > 0 {
		return bound[0]
	}
	if e.ctx != keys.ContextGlobal {
		if bound := m.registry.KeysFor(keys.ContextGlobal, e.act); len(bound) > 0 {
			return bound[0]
		}
	}
	return ""
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
