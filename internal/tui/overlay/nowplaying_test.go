package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestNowPlayingOverlay_BasicContent(t *testing.T) {
	n := NowPlayingOverlay{
		Artist:  "Test Artist",
		Title:   "Test Song",
		Elapsed: 83,
		Total:   296,
		State:   playback.StatePlaying,
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
		Artist: "A",
		Title:  "T",
		Album:  "Great Album",
		State:  playback.StatePlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "Great Album") {
		t.Error("expected album in output")
	}
}

func TestNowPlayingOverlay_AlbumAbsentWhenEmpty(t *testing.T) {
	n := NowPlayingOverlay{Artist: "The Artist", Title: "T", State: playback.StatePlaying}
	lines := strings.Split(ansi.Strip(n.Render(80, 24, newTestStyles())), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "The Artist" {
			continue
		}
		// Without an album the artist is followed by the spacer before the
		// progress line, not by an empty album row.
		if i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "" || !strings.Contains(lines[i+2], "0:00") {
			t.Fatalf("unexpected layout after artist:\n%s", strings.Join(lines, "\n"))
		}
		return
	}
	t.Fatal("artist line not found")
}

func TestNowPlayingOverlay_FullScreenLayout(t *testing.T) {
	cover := strings.TrimSuffix(strings.Repeat(strings.Repeat("▀", 20)+"\n", 10), "\n")
	n := NowPlayingOverlay{
		Artist: "Artist", Title: "Song", Album: "Record", State: playback.StatePlaying,
		Elapsed: 30, Total: 120,
		Cover: cover, CoverCols: 20, CoverRows: 10,
		Details: "2:00 · played 3 times", Modes: "Shuffle",
		Progress: func(width int) string { return strings.Repeat("=", width) },
		UpNext:   []string{"Next One", "Next Two"},
		Reason:   "Because you love it",
	}
	for _, size := range [][2]int{{100, 28}, {72, 16}, {60, 12}} {
		out := n.Render(size[0], size[1], newTestStyles())
		plain := ansi.Strip(out)
		lines := strings.Split(plain, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%dx%d: %d lines overflow", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Fatalf("%dx%d: line %q is %d wide", size[0], size[1], line, w)
			}
		}
		for _, want := range []string{"Song", "Artist", "0:30", "2:00", "Playing", "esc close"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%dx%d: missing %q in\n%s", size[0], size[1], want, plain)
			}
		}
	}
	plain := ansi.Strip(n.Render(100, 28, newTestStyles()))
	for _, want := range []string{"▀▀▀▀", "Up next", "1  Next One", "Why Auto-DJ chose it", "Because you love it", "Shuffle", "===="} {
		if !strings.Contains(plain, want) {
			t.Fatalf("roomy layout missing %q:\n%s", want, plain)
		}
	}
	// On a short screen the least important sections give way first.
	short := ansi.Strip(n.Render(100, 14, newTestStyles()))
	if strings.Contains(short, "Why Auto-DJ") && !strings.Contains(short, "Up next") {
		t.Fatalf("reason kept while up next was dropped:\n%s", short)
	}
}

func TestNowPlayingOverlay_PlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "▶") {
		t.Error("expected ▶ icon for playing state")
	}
}

func TestNowPlayingOverlay_PausedIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePaused}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "⏸") {
		t.Error("expected ⏸ icon for paused state")
	}
}

func TestNowPlayingOverlay_StoppedIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StateStopped}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "■") {
		t.Error("expected ■ icon for stopped state")
	}
	if strings.Contains(out, "▶") {
		t.Error("stopped state should not show ▶ icon")
	}
}

func TestNowPlayingOverlay_UnknownTrack(t *testing.T) {
	n := NowPlayingOverlay{State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(unknown title)") || !strings.Contains(out, "(unknown artist)") {
		t.Error("expected '(unknown title)' and '(unknown artist)' for empty artist/title")
	}
}

func TestNowPlayingOverlay_DismissHint(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "esc close") {
		t.Error("expected dismiss hint")
	}
}

// TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon verifies the bug fix
// where PlaybackIdle (the zero value of PlaybackState) used to fall through to
// the default switch arm and erroneously render the playing icon. Idle should
// render the stopped icon so the overlay never suggests playback is active when
// it isn't.
func TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StateIdle}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if strings.Contains(out, "▶") {
		t.Error("idle state must not render the playing icon")
	}
	if !strings.Contains(out, "■") {
		t.Error("idle state should fall back to the stopped icon")
	}
}
