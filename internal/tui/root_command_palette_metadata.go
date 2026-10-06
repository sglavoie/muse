package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (m Model) describeMetadataCommand(cmd palette.PaletteCommand) palette.PaletteCommand {
	if cmd.ID != "track.metadata.edit" {
		return cmd
	}
	ed, ok := m.metadataEditorForContext()
	cmd.Enabled = func() bool { return ok }
	if !ok {
		return cmd
	}
	n := len(ed.Hashes())
	tracks := fmt.Sprintf("%d %s", n, text.Pluralize(n, "track", "tracks"))
	switch ed.Mode() {
	case metadata.ModeRenameArtist:
		cmd.Detail = fmt.Sprintf("Rename artist across %s: %s", tracks, ed.Artist())
	case metadata.ModeRenameAlbum:
		cmd.Detail = fmt.Sprintf("Rename album across %s: %s", tracks, ed.Album())
	default:
		if n > 0 {
			cmd.Detail = "Edit metadata for " + tracks
		} else {
			cmd.Detail = "Edit metadata: " + text.TrackDisplay(ed.Artist(), ed.Title(), "")
		}
	}
	return cmd
}
