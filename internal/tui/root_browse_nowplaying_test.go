package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestBrowseNowPlayingUsesAdvertisedSongFromAnyView(t *testing.T) {
	for _, state := range []playback.State{playback.StatePlaying, playback.StatePaused} {
		m := newModelWithTracks()
		m.playback.currentHash = "h4" // Bob, while the focused Library row is Alice.
		m.playback.state = state
		for _, view := range viewOrder {
			m.ActiveView = view
			command := m.browseNowPlayingCommand()
			if !command.Enabled() || !strings.Contains(command.Detail, "Bob") {
				t.Fatalf("view %v: missing playing song: %+v", view, command)
			}
			// Advancing while the palette is open must not change its target.
			m.playback.currentHash = "h1"
			next, _ := m.Update(command.Exec()())
			opened := next.(Model)
			if opened.overlay.goTo == nil {
				t.Fatal("missing navigation popup")
			}
			_, pick := opened.overlay.goTo.HandleKey("a")
			if got := pick().(overlay.GoToPickedMsg).Value; got != "Bob" {
				t.Fatalf("navigated to %q, want Bob", got)
			}
			next, _ = opened.Update(pick())
			jumped := next.(Model)
			artist, ok := jumped.browse.Artists.FocusedArtist()
			if !ok || artist != "Bob" || jumped.ActiveView != viewBrowse || jumped.browse.Mode != browse.BrowseModeArtists {
				t.Fatal("artist shortcut did not focus the playing artist")
			}
			m.playback.currentHash = "h4"
		}
	}
}

func TestBrowseNowPlayingUnavailableAndStaleTargets(t *testing.T) {
	m := newModelWithTracks()
	if m.browseNowPlayingCommand().Enabled() {
		t.Fatal("idle command enabled")
	}
	m.playback.state = playback.StatePaused
	for _, hash := range []string{"", "removed"} {
		m.playback.currentHash = hash
		if m.browseNowPlayingCommand().Enabled() {
			t.Fatal("missing library track enabled")
		}
	}
	m = m.browseNowPlaying("removed")
	if m.overlay.goTo != nil || m.statusBar.TopNotification().Msg == "" {
		t.Fatal("stale target should report why it cannot open")
	}
}
