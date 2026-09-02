package statusbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestRenderElapsedTimeDisplay(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{Artist: "Artist", Title: "Title", PlaybackState: playback.StatePlaying, Elapsed: "01:23", Total: "04:56"}, 0)
	if !strings.Contains(out, "01:23 / 04:56") {
		t.Errorf("expected elapsed/total in output: got %q", out)
	}
}

func TestRenderNoTimeDisplayWhenBothZero(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{Artist: "Artist", Title: "Title", PlaybackState: playback.StatePlaying}, 0)
	if strings.Contains(out, " / ") {
		t.Errorf("unexpected time display when elapsed/total empty: got %q", out)
	}
}

func TestRenderTimeDisplayOnLine2(t *testing.T) {
	lines := renderStatusBarLines(t, StatusBar{Artist: "Artist", Title: "A Very Long Track Title That Should Be Truncated", PlaybackState: playback.StatePlaying, Elapsed: "01:23", Total: "04:56", ElapsedSecs: 83, TotalSecs: 296}, 60)
	if !strings.Contains(lines[1], "01:23 / 04:56") {
		t.Errorf("time display should be on line 2: got %q", lines[1])
	}
	if !strings.Contains(lines[0], "…") {
		t.Errorf("title should be truncated on line 1: got %q", lines[0])
	}
	if width := ansi.StringWidth(lines[0]); width > 60 {
		t.Errorf("line 1 visual width exceeds 60: width=%d, got %q", width, lines[0])
	}
}

func TestRenderAlbumAndProgressBarOnLine2(t *testing.T) {
	lines := renderStatusBarLines(t, StatusBar{Artist: "Artist", Title: "Title", Album: "Album", PlaybackState: playback.StatePlaying, Elapsed: "0:30", Total: "1:00", ElapsedSecs: 30, TotalSecs: 60}, 40)
	if strings.Contains(lines[0], "Album") {
		t.Errorf("album should not appear on line 1: got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Album") {
		t.Errorf("album should appear on line 2: got %q", lines[1])
	}
	if !strings.ContainsAny(lines[1], "█░▏▎▍▌▋▊▉") {
		t.Errorf("line 2 should contain progress bar glyphs: got %q", lines[1])
	}
}
