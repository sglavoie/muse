package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestPaletteFileCommandsDescribeFocusedTrack(t *testing.T) {
	m := newModelWithTracks()
	m.ActiveView = viewQueue
	m.playback.currentHash, m.playback.state = "h2", playback.StatePlaying
	m.queue.Entries = []queue.Entry{
		{Path: "/music/focused.flac", Artist: "Focused Artist", Title: "Focused Song"},
		{Path: "/music/untagged.flac"},
	}
	for cursor, wantInfo := range []string{"Focused Artist – Focused Song", "/music/untagged.flac"} {
		m.queue.Cursor = cursor
		for id, want := range map[string]string{
			"track.copy.info": wantInfo,
			"track.copy.path": m.queue.Entries[cursor].Path,
			"track.reveal":    m.queue.Entries[cursor].Path,
		} {
			command := polishCommand(t, m, id)
			if !command.IsEnabled() || command.Detail != want {
				t.Errorf("%s: enabled=%v detail=%q, want %q", id, command.IsEnabled(), command.Detail, want)
			}
		}
	}
	m.ActiveView, m.browse.Mode = viewBrowse, browse.BrowseModeArtists
	for _, id := range []string{"track.copy.info", "track.copy.path", "track.reveal"} {
		command := polishCommand(t, m, id)
		if command.IsEnabled() || command.Detail != "" || command.DisabledReason != "Focus a track first" {
			t.Errorf("collection-level command has a track target: %+v", command)
		}
	}
}
