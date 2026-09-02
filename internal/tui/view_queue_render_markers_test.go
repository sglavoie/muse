package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueRenderMissingEntryDim verifies that a missing entry is rendered with
// dim styling and shows the "[missing]" label.
func TestQueueRenderMissingEntryDim(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Gone", Artist: "Nobody", DurationMs: 0, Missing: true, Position: 1000},
	})
	s := styles.NewStyles(true)
	out := v.render(s, -1, playback.StateIdle, nil, nil, nil)
	if !strings.Contains(out, "[missing]") {
		t.Error("missing entry must contain '[missing]' label")
	}
	dimSample := styledText(s.Dim, "x")
	if ansi.Strip(dimSample) == dimSample {
		return
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), "[missing]") {
			if line == ansi.Strip(line) {
				t.Error("missing entry must be rendered with dim ANSI styling")
			}
			return
		}
	}
	t.Error("missing entry '[missing]' label not found in any output line")
}

// TestQueueRenderSelectedNonFocusedRowUsesAccent verifies that a selected entry
// that is not the focused row renders its selection marker with Accent styling.
func TestQueueRenderSelectedNonFocusedRowUsesAccent(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: false, Position: 1000},
		{Title: "Selected", Artist: "B", DurationMs: 60_000, Selected: true, Position: 2000},
	})
	v.cursor = 0
	s := styles.NewStyles(true)
	out := v.render(s, -1, playback.StateIdle, nil, nil, nil)

	want := styledText(s.AccentMark.Background(s.SelectionBg), "●")
	if ansi.Strip(want) == want {
		return
	}
	if !strings.Contains(out, want) {
		t.Errorf("selected non-focused row must contain accent-styled marker %q; got:\n%s", want, out)
	}
}

// TestQueueRenderSelectedEntryShowsMarker verifies that a bulk-selected queue
// entry is rendered with a "●" selection marker.
func TestQueueRenderSelectedEntryShowsMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false},
		{Title: "Selected", Artist: "B", DurationMs: 120_000, Selected: true},
	})
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	if !strings.Contains(out, "●") {
		t.Fatal("selected queue entry must show a ● marker")
	}
}

// TestQueueRenderUnselectedEntryHasNoMarker verifies that unselected queue
// entries do not contain a selection marker glyph.
func TestQueueRenderUnselectedEntryHasNoMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false}})
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	if strings.Contains(out, "●") {
		t.Fatal("unselected queue entry must not show a ● marker")
	}
}

// TestQueueRenderNowPlayingMarker verifies that the currently playing track
// gets a "▶" marker in the rendered output.
func TestQueueRenderNowPlayingMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Not Playing", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Playing", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0
	out := v.render(styles.NewStyles(true), 1, playback.StatePlaying, nil, nil, nil)
	stripped := ansi.Strip(out)

	lines := strings.Split(stripped, "\n")
	var playingLine string
	for _, l := range lines {
		if strings.Contains(l, "Playing") && !strings.Contains(l, "Not Playing") {
			playingLine = l
			break
		}
	}
	if playingLine == "" {
		t.Fatal("playing entry must appear in the output")
	}
	if !strings.Contains(playingLine, "▶") {
		t.Errorf("now-playing entry must contain ▶ marker; got: %q", playingLine)
	}
}

// TestQueueRenderNonPlayingEntryHasNoPlayMarker verifies that entries that are
// not currently playing do not get a "▶" marker.
func TestQueueRenderNonPlayingEntryHasNoPlayMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000}})
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	stripped := ansi.Strip(out)

	for l := range strings.SplitSeq(stripped, "\n") {
		if strings.Contains(l, "Normal") {
			if strings.Contains(l, "▶") {
				t.Errorf("non-playing entry must not contain ▶ marker; got: %q", l)
			}
			return
		}
	}
	t.Fatal("entry must appear in the output")
}
