package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/palette"
)

func (m Model) describePlaylistCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	switch cmd.ID {
	case "playlist.rename", "playlist.delete", "playlist.smart.edit", "playlist.enqueue_all":
	default:
		return cmd
	}
	if m.ActiveView != viewPlaylists {
		return unavailableQueueCommand(cmd, "Open Playlists first")
	}
	if cmd.ID == "playlist.enqueue_all" {
		_, name, _, _, ok := m.resolvePlaylistForEnqueue()
		if !ok {
			return unavailableQueueCommand(cmd, "Focus or open a playlist first")
		}
		cmd.Detail = "Enqueue entire playlist: " + name
		return cmd
	}
	if !m.playlists.AtListLevel() {
		return unavailableQueueCommand(cmd, "Return to the playlist list first")
	}
	pl, ok := m.playlists.FocusedPlaylist()
	if !ok {
		return unavailableQueueCommand(cmd, "Focus a playlist first")
	}
	if cmd.ID == "playlist.smart.edit" && !pl.IsSmart {
		return unavailableQueueCommand(cmd, "Focus a smart playlist first")
	}
	cmd.Detail = "Playlist: " + pl.Name
	return cmd
}

type refreshSmartPlaylistMsg struct{}

func (m Model) canRefreshSmartPlaylist() bool {
	return m.ActiveView == viewPlaylists && !m.playlists.AtListLevel() &&
		m.playlists.SmartQuery != nil && m.smartEval != nil
}

func (m Model) refreshSmartPlaylistCommand() palette.PaletteCommand {
	cmd := paletteMessageCmd("playlist.smart.refresh", "list", "Refresh smart playlist", m.paletteShortcutFor(paletteEntry{act: keys.ActionPlaylistRename, ctx: keys.ContextPlaylists}), refreshSmartPlaylistMsg{})
	cmd.Enabled = m.canRefreshSmartPlaylist
	cmd.DisabledReason = "Open a smart playlist first"
	if m.canRefreshSmartPlaylist() {
		_, name, _, _, _ := m.resolvePlaylistForEnqueue()
		cmd.Detail = "Re-evaluate rules: " + name
	}
	return cmd
}

func (m Model) handleRefreshSmartPlaylist() (Model, tea.Cmd) {
	if !m.canRefreshSmartPlaylist() {
		return m, nil
	}
	m, cmd, _ := m.handlePlaylistKeyAction(keys.ActionPlaylistRename)
	return m, cmd
}
