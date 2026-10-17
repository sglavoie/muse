package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/palette"
)

// focusedTrackForPalette returns the path, artist, and title of the track
// currently under the cursor, regardless of which view is active. Returns
// found=false when the cursor is not on a track row (e.g., artist/album level).
func (m Model) focusedTrackForPalette() (path, artist, title string, found bool) {
	if m.ActiveView == viewQueue {
		entries := m.queue.FilteredEntries()
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(entries) {
			e := entries[m.queue.Cursor]
			return e.Path, e.Artist, e.Title, true
		}
		return "", "", "", false
	}
	track, ok := m.focusedTrackForPreview()
	return track.Path, track.Artist, track.Title, ok
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
		Relevant:       e.ctx != keys.ContextGlobal && e.ctx == m.activeViewContext(),
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
