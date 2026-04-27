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

	cmds = append(cmds, PaletteCommand{
		Name: "Toggle radio mode (Auto-DJ)",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return commandPaletteActionMsg{action: actionRadioToggle} }
		},
	})

	trackFocused := func() bool { _, _, _, found := m.focusedTrackForPalette(); return found }

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

	cmds = append(cmds, PaletteCommand{
		Name: "Filter by tag…",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return openTagFilterPickerMsg{} }
		},
		Enabled: func() bool { return len(nonEmptyTags(m.tags, m.tagsView.tagCounts)) > 0 },
	})
	if m.activeTagFilter != nil {
		cmds = append(cmds, PaletteCommand{
			Name: "Clear tag filter",
			Exec: func() tea.Cmd {
				return func() tea.Msg { return clearTagFilterMsg{} }
			},
		})
	}

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

	for _, tag := range m.tags {
		if m.tagsView.tagCounts != nil && m.tagsView.tagCounts[tag.ID] == 0 {
			continue
		}
		t := tag
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
	case viewTags:
		if track, ok := m.tagsView.focusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}
	}

	return "", "", "", false
}
