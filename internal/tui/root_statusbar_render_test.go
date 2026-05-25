package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestViewStatusBarShowsConfigPathOnFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New(context.Background(), WithConfigPath(configPath, true))
	assertViewContains(t, m, configPath)
}

func TestViewStatusBarHidesConfigPathAfterFirstRun(t *testing.T) {
	configPath := "/home/user/.config/muse/config.toml"
	m := New(context.Background(), WithConfigPath(configPath, false))
	assertViewNotContains(t, m, configPath)
}

func TestStatusBarReceivesTrackInfoBeforeRender(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{{Path: "/a.flac", Display: "The Artist – The Song", Artist: "The Artist", Title: "The Song"}}

	nm, _ := updateModel(m, PlayQueueIndexMsg{Index: 0})

	assertViewContains(t, nm, "The Artist")
	assertViewContains(t, nm, "The Song")
	assertViewContains(t, nm, "Queue")
}

func TestStatusBarShowsPlaybackStateIndicator(t *testing.T) {
	tests := []struct {
		state PlaybackState
		want  string
	}{
		{PlaybackPlaying, "▶"},
		{PlaybackPaused, "⏸"},
		{PlaybackStopped, "■"},
	}

	for _, tc := range tests {
		sb := StatusBar{artist: "A", title: "T", playbackState: tc.state}
		out := sb.render(0, styles.Styles{})
		if !strings.Contains(out, tc.want) {
			t.Fatalf("state %v: expected %q in status bar; got: %s", tc.state, tc.want, out)
		}
	}
}

func TestStatusBarRendersElapsedTime(t *testing.T) {
	p := &stubElapsedReader{elapsed: 83.0, total: 245.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"

	nm, _ := updateModel(m, tickMsg{})
	assertViewContains(t, nm, "1:23 / 4:05")
}

func TestStatusBarHidesStaleQueuePosition(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Path: "/a.flac"}, {ID: 2, Path: "/b.flac"}}
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = len(m.queue.entries)
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.termWidth = 200

	out := renderedStatusBar(m)
	if strings.Contains(out, "3 of 2") {
		t.Fatalf("status bar must not render stale queue position; got:\n%s", out)
	}
}

func TestStatusBarNoTimeDisplayWhenZero(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"

	assertViewNotContains(t, m, " / ")
}
