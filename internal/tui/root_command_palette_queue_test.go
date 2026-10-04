package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/palette"
)

func queuePaletteCommand(t *testing.T, m Model, id string) palette.PaletteCommand {
	t.Helper()
	for _, cmd := range m.BuildPaletteCommands() {
		if cmd.ID == id {
			return cmd
		}
	}
	t.Fatalf("missing command %s", id)
	return palette.PaletteCommand{}
}

func TestQueuePaletteDescribesActualTargets(t *testing.T) {
	m := newModelWithTracks()
	m.library.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}, "stale": {}}
	// A text filter hides the selection but the enqueue handler still uses it.
	m.library.Filter = filter.State{Text: "Charlie"}
	m.library = m.library.RebuildFilterCache()
	want := map[string]string{
		"queue.enqueue.focused": "Append 2 selected tracks to queue",
		"queue.play_next":       "Play next: Track D1 — Charlie · focused track only",
		"queue.enqueue.replace": "Clear queue and enqueue: Track D1 — Charlie · focused track only",
	}
	for id, detail := range want {
		cmd := queuePaletteCommand(t, m, id)
		if !cmd.IsEnabled() || cmd.Detail != detail {
			t.Errorf("%s: enabled=%v, detail=%q; want %q", id, cmd.IsEnabled(), cmd.Detail, detail)
		}
	}
	m.library.SelectedHashes = nil
	cmd := queuePaletteCommand(t, m, "queue.enqueue.focused")
	if cmd.Detail != "Append to queue: Track D1 — Charlie" {
		t.Fatalf("reopened command retained old selection: %+v", cmd)
	}
	m.library.SelectedHashes = map[string]struct{}{"h1": {}}
	if got := queuePaletteCommand(t, m, "queue.enqueue.focused").Detail; got != "Append 1 selected track to queue" {
		t.Fatalf("single selection detail = %q", got)
	}
}

func TestQueuePaletteUnavailableTargets(t *testing.T) {
	for _, id := range []string{"queue.enqueue.focused", "queue.play_next", "queue.enqueue.replace"} {
		m := newModelWithTracks()
		m = pressEnter(typeText(pressSlash(m), "no-such-track"))
		cmd := queuePaletteCommand(t, m, id)
		if cmd.IsEnabled() || cmd.DisabledReason != "Focus a track first" {
			t.Errorf("%s should explain empty results: %+v", id, cmd)
		}
	}
	m := newModelWithTracks()
	m.library.SelectedHashes = map[string]struct{}{"stale": {}}
	cmd := queuePaletteCommand(t, m, "queue.enqueue.focused")
	if cmd.IsEnabled() || cmd.DisabledReason != "No selected tracks available" {
		t.Fatalf("stale selection should not fall back to focus: %+v", cmd)
	}
	m.library.SelectedHashes = map[string]struct{}{"h1": {}}
	m = pressEnter(typeText(pressSlash(m), "no-such-track"))
	if !queuePaletteCommand(t, m, "queue.enqueue.focused").IsEnabled() {
		t.Fatal("a valid hidden selection should remain enqueueable")
	}
}

func TestQueuePaletteCollectionAndViewGates(t *testing.T) {
	for _, view := range []viewID{viewBrowse, viewPlaylists, viewFavorites, viewTags} {
		m := New(t.Context())
		m.ActiveView = view
		for _, id := range []string{"queue.play_next", "queue.enqueue.replace"} {
			cmd := queuePaletteCommand(t, m, id)
			if cmd.IsEnabled() || cmd.DisabledReason != "Focus a track first" {
				t.Errorf("view %v, %s should require a track: %+v", view, id, cmd)
			}
		}
	}
	m := newRichFixture()
	m.ActiveView = viewQueue
	for _, id := range []string{"queue.enqueue.focused", "queue.play_next", "queue.enqueue.replace"} {
		cmd := queuePaletteCommand(t, m, id)
		if cmd.IsEnabled() || !strings.HasPrefix(cmd.DisabledReason, "Open ") {
			t.Errorf("%s lost its original view gate: %+v", id, cmd)
		}
	}
}

func TestQueuePaletteBrowseAndMixFocus(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Level = artists.TrackLevel
	m.browse.Artists.Tracks = sampleLibraryTracks()[:2]
	m.browse.Artists.Cursor = 0
	for _, id := range []string{"queue.play_next", "queue.enqueue.replace"} {
		cmd := queuePaletteCommand(t, m, id)
		if !cmd.IsEnabled() || !strings.Contains(cmd.Detail, "Track A1 — Alice") {
			t.Fatalf("Browse track target missing: %+v", cmd)
		}
	}
	m = m.switchToView(viewMixes)
	if cmd := queuePaletteCommand(t, m, "queue.enqueue.focused"); cmd.IsEnabled() {
		t.Fatal("mix list should require opening a track list")
	}
	m.mixes.OpenMix(m.mixes.Mixes[0].ID)
	m = m.handleMixLoadedMsg(m.loadMixCmd(m.mixes.ActiveMix)().(mixLoadedMsg))
	cmd := queuePaletteCommand(t, m, "queue.enqueue.focused")
	track := m.mixes.FocusedTrack()
	if track == nil || !cmd.IsEnabled() || cmd.Detail != "Append to queue: "+track.Title+" — "+track.Artist {
		t.Fatalf("mix focus not described: %+v", cmd)
	}
}
