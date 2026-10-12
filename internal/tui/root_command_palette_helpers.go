package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/palette"
)

// focusedTrackForPalette returns the path, artist, and title of the track
// currently under the cursor, regardless of which view is active. Returns
// found=false when the cursor is not on a track row (e.g., artist/album level).
func (m Model) focusedTrackForPalette() (path, artist, title string, found bool) {
	switch m.ActiveView {
	case viewLibrary:
		if track, ok := m.library.Panel().FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewQueue:
		filtered := m.queue.FilteredEntries()
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
			e := filtered[m.queue.Cursor]
			return e.Path, e.Artist, e.Title, true
		}

	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				if track, ok := m.browse.Artists.FocusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				if track, ok := m.browse.Albums.FocusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level == gentui.TrackLevel {
				if track, ok := m.browse.Genres.FocusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}

	case viewFavorites:
		switch m.favorites.Level {
		case favtui.ListLevel:
			if m.favorites.Category == favtui.CategoryTracks {
				if track, ok := m.favorites.FocusedTrack(); ok {
					return track.Path, track.Artist, track.Title, true
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.FocusedTrack(); ok {
				return track.Path, track.Artist, track.Title, true
			}
		}

	case viewTags:
		if track, ok := m.tagsView.FocusedTrack(); ok {
			return track.Path, track.Artist, track.Title, true
		}
	case viewMixes:
		if t := m.mixes.FocusedTrack(); t != nil {
			return t.Path, t.Artist, t.Title, true
		}
	case viewInsights:
		if t := m.insights.FocusedTrack(); t != nil {
			return t.Path, t.Artist, t.Title, true
		}
	}

	return "", "", "", false
}

// paletteCommandFor builds the palette command for a catalog entry. The
// action is dispatched through the command palette action pipeline and the
// shortcut is derived from the live registry, so a rebind cannot leave a stale
// hint behind.
func (m Model) paletteCommandFor(e paletteEntry) palette.PaletteCommand {
	cmd := palette.PaletteCommand{
		ID:             e.id,
		Name:           e.name,
		Category:       e.category,
		Shortcut:       m.paletteShortcutFor(e),
		Pinned:         e.pin,
		DisabledReason: e.disabledReason,
		Exec:           func() tea.Cmd { return func() tea.Msg { return commandPaletteActionMsg{action: e.act, view: e.view} } },
	}
	if e.enabled != nil {
		// e.enabled is evaluated here, against the pre-switch model, because
		// the row is rendered before the switch happens.
		cmd.Enabled = func() bool { return e.enabled(m) }
	}
	return m.describeMetadataCommand(cmd)
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
func paletteMessageCmd(id, category, name, shortcut string, msg tea.Msg) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:       id,
		Name:     name,
		Category: category,
		Shortcut: shortcut,
		Exec:     func() tea.Cmd { return func() tea.Msg { return msg } },
	}
}

// configTogglePalette returns a PaletteCommand that sends a configToggleMsg
// for the given audio setting field.
func configTogglePalette(id, category, name string, field configToggleField) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:       id,
		Name:     name,
		Category: category,
		Exec:     func() tea.Cmd { return func() tea.Msg { return configToggleMsg{field: field} } },
	}
}

func revealFilePaletteName(goos string) string {
	if goos == "darwin" {
		return "Reveal in Finder"
	}
	return "Open containing folder"
}
