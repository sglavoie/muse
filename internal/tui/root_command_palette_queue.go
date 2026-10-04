package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/text"
)

// describeQueueCommand mirrors the existing handlers' target rules without
// widening their view gates or changing what an action will enqueue.
func (m Model) describeQueueCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	switch cmd.ID {
	case "queue.enqueue.focused", "queue.play_next", "queue.enqueue.replace":
	default:
		return cmd
	}
	if !cmd.IsEnabled() {
		return cmd
	}
	if cmd.ID == "queue.enqueue.focused" && m.ActiveView == viewLibrary && len(m.library.SelectedHashes) > 0 {
		return m.describeSelectedQueueCommand(cmd)
	}
	return m.describeFocusedQueueCommand(cmd)
}

func (m Model) describeSelectedQueueCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	count := len(m.bulkLibraryTracks())
	if count == 0 {
		return unavailableQueueCommand(cmd, "No selected tracks available")
	}
	cmd.Detail = fmt.Sprintf("Append %d selected %s to queue", count, text.Pluralize(count, "track", "tracks"))
	return cmd
}

func (m Model) describeFocusedQueueCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	_, artist, title, found := m.focusedTrackForPalette()
	if !found {
		return unavailableQueueCommand(cmd, "Focus a track first")
	}
	target := title + " — " + artist
	switch cmd.ID {
	case "queue.enqueue.focused":
		cmd.Detail = "Append to queue: " + target
	case "queue.play_next":
		cmd.Detail = "Play next: " + target
	case "queue.enqueue.replace":
		cmd.Detail = "Clear queue and enqueue: " + target
	}
	if m.activeViewSelectionCount() > 0 {
		cmd.Detail += " · focused track only"
	}
	return cmd
}

func unavailableQueueCommand(cmd palette.PaletteCommand, reason string) palette.PaletteCommand {
	cmd.Enabled = func() bool { return false }
	cmd.DisabledReason = reason
	return cmd
}
