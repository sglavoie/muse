package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestNowPlayingOverlay_BasicContent(t *testing.T) {
	n := NowPlayingOverlay{
		artist:  "Test Artist",
		title:   "Test Song",
		elapsed: 83,
		total:   296,
		state:   playback.StatePlaying,
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
		state:  playback.StatePlaying,
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
		state:  playback.StatePlaying,
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
	n := NowPlayingOverlay{artist: "A", title: "T", state: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "▶") {
		t.Error("expected ▶ icon for playing state")
	}
}

func TestNowPlayingOverlay_PausedIcon(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: playback.StatePaused}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "⏸") {
		t.Error("expected ⏸ icon for paused state")
	}
}

func TestNowPlayingOverlay_StoppedIcon(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: playback.StateStopped}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "■") {
		t.Error("expected ■ icon for stopped state")
	}
	if strings.Contains(out, "▶") {
		t.Error("stopped state should not show ▶ icon")
	}
}

func TestNowPlayingOverlay_UnknownTrack(t *testing.T) {
	n := NowPlayingOverlay{state: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(unknown title)") || !strings.Contains(out, "(unknown artist)") {
		t.Error("expected '(unknown title)' and '(unknown artist)' for empty artist/title")
	}
}

func TestNowPlayingOverlay_DismissHint(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "I or esc to dismiss") {
		t.Error("expected dismiss hint")
	}
}

// TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon verifies the bug fix
// where PlaybackIdle (the zero value of PlaybackState) used to fall through to
// the default switch arm and erroneously render the playing icon. Idle should
// render the stopped icon so the overlay never suggests playback is active when
// it isn't.
func TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{artist: "A", title: "T", state: playback.StateIdle}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if strings.Contains(out, "▶") {
		t.Error("idle state must not render the playing icon")
	}
	if !strings.Contains(out, "■") {
		t.Error("idle state should fall back to the stopped icon")
	}
}
