package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestNowPlayingOverlay_BasicContent(t *testing.T) {
	n := NowPlayingOverlay{
		artist:  "Test Artist",
		title:   "Test Song",
		elapsed: 83,
		total:   296,
		state:   PlaybackPlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))

	if !strings.Contains(out, "Now Playing") {
		t.Error("expected 'Now Playing' header")
	}
	if !strings.Contains(out, "Test Artist") {
		t.Error("expected artist in output")
	}
	if !strings.Contains(out, "Test Song") {
		t.Error("expected title in output")
	}
	if !strings.Contains(out, "1:23") || !strings.Contains(out, "4:56") {
		t.Error("expected formatted elapsed/total times")
	}
}

func TestNowPlayingOverlay_AlbumShown(t *testing.T) {
	n := NowPlayingOverlay{
		artist: "A",
		title:  "T",
		album:  "Great Album",
		state:  PlaybackPlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "Great Album") {
		t.Error("expected album in output")
	}
}

func TestNowPlayingOverlay_AlbumAbsentWhenEmpty(t *testing.T) {
	n := NowPlayingOverlay{
		artist: "A",
		title:  "T",
		state:  PlaybackPlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	// Count content rows between borders — should not have an extra row for album.
	lines := strings.Split(out, "\n")
	contentRows := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "│") && strings.HasSuffix(trimmed, "│") {
			// Skip title row and blank rows.
			inner := strings.TrimSpace(trimmed[len("│") : len(trimmed)-len("│")])
			if inner != "" && !strings.Contains(inner, "Now Playing") {
				contentRows++
			}
		}
	}
	// Without album: title + artist + play count + progress = 4 content rows.
	if contentRows != 4 {
		t.Errorf("expected 4 content rows without album, got %d", contentRows)
	}
}

func TestNowPlayingOverlay_PlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: PlaybackPlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "▶") {
		t.Error("expected ▶ icon for playing state")
	}
}

func TestNowPlayingOverlay_PausedIcon(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: PlaybackPaused}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "⏸") {
		t.Error("expected ⏸ icon for paused state")
	}
}

func TestNowPlayingOverlay_UnknownTrack(t *testing.T) {
	n := NowPlayingOverlay{state: PlaybackPlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(unknown title)") || !strings.Contains(out, "(unknown artist)") {
		t.Error("expected '(unknown title)' and '(unknown artist)' for empty artist/title")
	}
}

func TestNowPlayingOverlay_DismissHint(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: PlaybackPlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "I or esc to dismiss") {
		t.Error("expected dismiss hint")
	}
}
