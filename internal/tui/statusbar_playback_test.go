package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderElapsedTimeDisplay(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "Title", playbackState: PlaybackPlaying, elapsed: "01:23", total: "04:56"}, 0)
	if !strings.Contains(out, "01:23 / 04:56") {
		t.Errorf("expected elapsed/total in output: got %q", out)
	}
}

func TestRenderNoTimeDisplayWhenBothZero(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "Title", playbackState: PlaybackPlaying}, 0)
	if strings.Contains(out, " / ") {
		t.Errorf("unexpected time display when elapsed/total empty: got %q", out)
	}
}

func TestRenderTimeDisplayOnLine2(t *testing.T) {
	lines := renderStatusBarLines(t, StatusBar{artist: "Artist", title: "A Very Long Track Title That Should Be Truncated", playbackState: PlaybackPlaying, elapsed: "01:23", total: "04:56", elapsedSecs: 83, totalSecs: 296}, 60)
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
	lines := renderStatusBarLines(t, StatusBar{artist: "Artist", title: "Title", album: "Album", playbackState: PlaybackPlaying, elapsed: "0:30", total: "1:00", elapsedSecs: 30, totalSecs: 60}, 40)
	if strings.Contains(lines[0], "Album") {
		t.Errorf("album should not appear on line 1: got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Album") {
		t.Errorf("album should appear on line 2: got %q", lines[1])
	}
	if !strings.Contains(lines[1], "█") && !strings.Contains(lines[1], "░") {
		t.Errorf("line 2 should contain progress bar glyphs: got %q", lines[1])
	}
}
