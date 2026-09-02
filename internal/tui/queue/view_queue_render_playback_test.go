package queue

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueRenderPlayedRowDimGlyph verifies that an entry before the
// now-playing index renders with the played-tier ╵ left glyph and dim styling.
func TestQueueRenderPlayedRowDimGlyph(t *testing.T) {
	v := makeQueue(120, []Entry{
		{Title: "Past", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Now", Artist: "B", Hash: "bbb", DurationMs: 60_000},
		{Title: "Next", Artist: "C", Hash: "ccc", DurationMs: 60_000},
	})
	v.Cursor = 1
	s := styles.NewStyles(true)
	out := v.Render(s, 1, playback.StatePlaying, nil, nil, nil)

	var pastLine, nextLine string
	for l := range strings.SplitSeq(out, "\n") {
		stripped := ansi.Strip(l)
		switch {
		case strings.Contains(stripped, "Past"):
			pastLine = l
		case strings.Contains(stripped, "Next"):
			nextLine = l
		}
	}
	if pastLine == "" {
		t.Fatal("played entry must appear in output")
	}
	if !strings.Contains(ansi.Strip(pastLine), "╵") {
		t.Errorf("played row must contain ╵ glyph; got: %q", ansi.Strip(pastLine))
	}
	if nextLine != "" && strings.Contains(ansi.Strip(nextLine), "╵") {
		t.Errorf("upcoming row must not contain ╵ glyph; got: %q", ansi.Strip(nextLine))
	}

	playedRef := s.PlayedRow.Render("REF")
	if ansi.Strip(playedRef) == playedRef {
		return
	}
	playedPrefix, _, _ := strings.Cut(playedRef, "REF")
	if !strings.Contains(pastLine, playedPrefix) {
		t.Errorf("played row must use PlayedRow style; got: %q", pastLine)
	}
}

// TestQueueRenderUpcomingRowNotDimmed verifies that an entry after the
// now-playing index keeps full-contrast styling (no PlayedRow override).
func TestQueueRenderUpcomingRowNotDimmed(t *testing.T) {
	v := makeQueue(120, []Entry{
		{Title: "Now", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Upcoming", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	v.Cursor = 0
	s := styles.NewStyles(true)
	out := v.Render(s, 0, playback.StatePlaying, nil, nil, nil)

	var line string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(l), "Upcoming") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("upcoming entry must appear in output")
	}
	if strings.Contains(ansi.Strip(line), "╵") {
		t.Errorf("upcoming row must not contain ╵ glyph; got: %q", ansi.Strip(line))
	}
}

// TestQueueRenderNoPlayedTierWhenIdle verifies that with no now-playing index
// (-1), no rows are treated as played.
func TestQueueRenderNoPlayedTierWhenIdle(t *testing.T) {
	v := makeQueue(120, []Entry{
		{Title: "One", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Two", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	out := v.Render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "╵") {
		t.Errorf("no rows should be dimmed when idle; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderSelectedPlayedShowsSelectionMarker verifies that a row which
// is both selected and in the played tier keeps the selection "●" marker
// (selection precedence wins over the played ╵ glyph).
func TestQueueRenderSelectedPlayedShowsSelectionMarker(t *testing.T) {
	v := makeQueue(120, []Entry{
		{Title: "PastSel", Artist: "A", Hash: "aaa", DurationMs: 60_000, Selected: true},
		{Title: "Now", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	v.Cursor = 1
	s := styles.NewStyles(true)
	out := v.Render(s, 1, playback.StatePlaying, nil, nil, nil)

	var line string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(l), "PastSel") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("selected played entry must appear in output")
	}
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "●") {
		t.Errorf("selected+played row must keep ● marker; got: %q", stripped)
	}
	if strings.Contains(stripped, "╵") {
		t.Errorf("selected+played row must not show ╵ marker (selection wins); got: %q", stripped)
	}
}
