package library

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
)

// TestRenderColumnsDoNotOverflow verifies that each row in the rendered output
// has exactly the expected byte length at a given terminal width.
func TestRenderColumnsDoNotOverflow(t *testing.T) {
	for _, width := range []int{40, 80, 120, 160} {
		v := newRenderView(sampleTracks)
		v.Width = width
		v.Height = 0
		assertRenderedLineWidths(t, v.Render(), width)
	}
}

func TestRenderDegenerateColumnsDoNotOverflow(t *testing.T) {
	for _, width := range []int{1, 2, 3, 5, 24, 25} {
		v := newRenderView(sampleTracks)
		v.Width = width
		v.Height = 4
		assertRenderedLineWidths(t, v.Render(), width)
	}
}

func TestRenderNearDegenerateThresholdDoNotUnderflow(t *testing.T) {
	for _, width := range []int{26, 27} {
		v := newRenderView(sampleTracks)
		v.Width = width
		v.Height = 4
		assertRenderedLineWidths(t, v.Render(), width)
	}
}

// TestRenderEmptyIndex verifies the empty-library card surfaces its CTAs.
func TestRenderEmptyIndex(t *testing.T) {
	got := newRenderView(nil).Render()
	if !strings.Contains(got, "Library is empty") {
		t.Errorf("empty library must show title; got: %q", got)
	}
	for _, want := range []string{"ctrl+r", "rescan folders", "help"} {
		if !strings.Contains(got, want) {
			t.Errorf("empty library card missing CTA %q; got: %q", want, got)
		}
	}
	// There is no import action; the card must not advertise one.
	if strings.Contains(got, "import") {
		t.Errorf("empty library card advertises a nonexistent import action; got: %q", got)
	}
}

// TestRenderZeroWidthFallback verifies that a zero Width renders a simple
// "artist – title" format without panic.
func TestRenderZeroWidthFallback(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.Width = 0
	v.Height = 0

	got := v.Render()
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("zero-width render should not contain a column header row, got: %q", line)
		}
	}
}

// TestRenderSortOrderUnknownArtistLast verifies that "Unknown Artist" entries
// sort below named artists in the rendered output.
func TestRenderSortOrderUnknownArtistLast(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.Height = 0

	got := v.Render()
	alicePos := strings.Index(got, "Alice")
	unknownPos := strings.Index(got, "Unknown Artist")
	if alicePos == -1 || unknownPos == -1 {
		t.Fatalf("could not find expected artist names in render:\n%s", got)
	}
	if alicePos > unknownPos {
		t.Errorf("expected named artists before Unknown Artist; alice at %d, unknown at %d", alicePos, unknownPos)
	}
}

func TestRenderShowsDateAddedColumnWhenEnabled(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185, CreatedAt: 1711929600},
	}
	v := newRenderView(tracks)
	v.Width = 100
	v.Height = 10
	v.ShowDateAdded = true

	got := renderPlain(v)
	wantDate := time.Unix(1711929600, 0).Local().Format("2006-01-02")
	if !strings.Contains(got, "Added") {
		t.Fatalf("expected Added header in render, got:\n%s", got)
	}
	if !strings.Contains(got, wantDate) {
		t.Fatalf("expected formatted date in render, got:\n%s", got)
	}
}

func TestRenderDoesNotExceedConfiguredHeight(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.Height = 4

	lines := renderLines(v)
	if len(lines) != v.Height {
		t.Fatalf("rendered line count = %d, want height %d:\n%s", len(lines), v.Height, v.Render())
	}
	if !strings.Contains(ansi.Strip(lines[0]), "Library") {
		t.Fatalf("first line should be the state strip, got %q", lines[0])
	}
	if !strings.Contains(ansi.Strip(lines[1]), "Artist") {
		t.Fatalf("second line should be the column header, got %q", lines[1])
	}
}
