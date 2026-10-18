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
	if m.atEnqueueTrackLevel() && m.activeViewSelectionCount() > 0 {
		return m.describeSelectedQueueCommand(cmd)
	}
	return m.describeFocusedQueueCommand(cmd)
}

func (m Model) describeSelectedQueueCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	count := len(m.selectedEnqueueTracks())
	if count == 0 {
		return unavailableQueueCommand(cmd, "No selected tracks available")
	}
	switch cmd.ID {
	case "queue.play_next":
		cmd.Name = fmt.Sprintf("Play %d selected next", count)
		cmd.Detail = fmt.Sprintf("Play %d selected %s next", count, text.Pluralize(count, "track", "tracks"))
	case "queue.enqueue.replace":
		cmd.Name = fmt.Sprintf("Replace queue with %d selected", count)
		cmd.Detail = fmt.Sprintf("Replace queue with %d selected %s", count, text.Pluralize(count, "track", "tracks"))
	default:
		cmd.Detail = fmt.Sprintf("Append %d selected %s to queue", count, text.Pluralize(count, "track", "tracks"))
	}
	if hidden := count - m.visibleEnqueueSelectionCount(); hidden > 0 {
		cmd.Detail += fmt.Sprintf(" · %d hidden", hidden)
	}
	if cmd.ID == "queue.enqueue.replace" {
		cmd.Detail += " · undo restores previous queue"
	}
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
		cmd.Detail = "Replace queue with: " + target + " · undo restores previous queue"
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
