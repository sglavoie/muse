package tags

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderTagsAllZeroCountsShowsEmptyState(t *testing.T) {
	tags := tagRows("unused", "empty")
	v := View{
		Level: TagLevel,
		Width: 40,
		Tags:  tags,
		TagCounts: map[int64]int{
			tags[0].ID: 0,
			tags[1].ID: 0,
		},
	}

	out := v.Render()

	if strings.Contains(out, "(no matches)") {
		t.Fatalf("zero-count tags without a filter should render an empty state, got:\n%s", out)
	}
	if !strings.Contains(out, "No tagged tracks yet") {
		t.Fatalf("expected empty tagged-tracks message, got:\n%s", out)
	}
}

// TestRenderTagsEmptyStateSuggestsCreate guards that the empty Tags card
// offers a key the Tags view acts on: t tags the focused track, and the Tags
// list has no track to tag.
func TestRenderTagsEmptyStateSuggestsCreate(t *testing.T) {
	out := ansi.Strip(View{Level: TagLevel, Width: 60}.Render())
	if !strings.Contains(out, "create a tag") {
		t.Errorf("expected a create-a-tag CTA, got:\n%s", out)
	}
	if strings.Contains(out, "tag a track") {
		t.Errorf("empty Tags card must not suggest t, got:\n%s", out)
	}
}

func TestRenderTagsFilterNoMatchesStillShowsNoMatches(t *testing.T) {
	tags := tagRows("alpha")
	v := View{
		Level:      TagLevel,
		Width:      40,
		FilterText: "zzz",
		Tags:       tags,
		TagCounts:  tagCountsFor(tags, 1),
	}

	out := v.Render()

	if !strings.Contains(out, "(no matches)") {
		t.Fatalf("expected filter miss to render no matches, got:\n%s", out)
	}
}

func TestRenderTagsCreatingWithNoVisibleRowsDoesNotPanic(t *testing.T) {
	tags := tagRows("a")
	v := View{
		Level:         TagLevel,
		Width:         20,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags:          tags,
		TagCounts:     tagCountsFor(tags, 1),
		Creating:      true,
		CreateInput:   "new",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Render panicked with no visible rows while creating: %v", r)
		}
	}()

	_ = v.Render()
}

func TestRenderTagsWithNegativeScrollOffsetDoesNotPanic(t *testing.T) {
	tags := tagRows("alpha")
	v := View{
		Level:        TagLevel,
		Width:        20,
		Height:       10,
		ScrollOffset: -3,
		Tags:         tags,
		TagCounts:    tagCountsFor(tags, 1),
	}

	out := v.Render()

	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected render to clamp negative scroll and include first tag, got:\n%s", out)
	}
}

func TestRenderTagsWithHighScrollOffsetKeepsCursorVisible(t *testing.T) {
	tags := tagRows("alpha", "beta", "gamma")
	v := View{
		Level:        TagLevel,
		Width:        20,
		Height:       2,
		ScrollOffset: 50,
		Tags:         tags,
		TagCounts:    tagCountsFor(tags, 1),
	}

	out := v.Render()

	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected render to clamp high scroll and include focused tag, got:\n%s", out)
	}
}

func TestRenderTagsPreservesCountWhenNameTruncates(t *testing.T) {
	const width = 18
	tags := tagRows("a very very long tag name")
	v := View{
		Level:     TagLevel,
		Width:     width,
		Height:    2,
		Tags:      tags,
		TagCounts: map[int64]int{tags[0].ID: 42},
	}

	out := strings.Split(ansi.Strip(v.Render()), "\n")[1]

	if !strings.Contains(out, "42 tracks") {
		t.Fatalf("expected truncated tag row to preserve count suffix, got:\n%s", out)
	}
	if got := ansi.StringWidth(out); got != width {
		t.Fatalf("tag row width = %d, want %d; row=%q", got, width, out)
	}
}

func TestRenderTracksWithNegativeScrollOffsetDoesNotPanic(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       10,
		ScrollOffset: -2,
		Tracks:       trackRows("first"),
	}

	out := v.Render()

	if !strings.Contains(out, "first") {
		t.Fatalf("expected render to clamp negative scroll and include first track, got:\n%s", out)
	}
}

func TestRenderTracksWithHighScrollOffsetKeepsCursorVisible(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       5,
		ScrollOffset: 50,
		Tracks:       trackRows("first", "second", "third"),
	}

	out := v.Render()

	if !strings.Contains(out, "first") {
		t.Fatalf("expected render to clamp high scroll and include focused track, got:\n%s", out)
	}
}

func TestRenderTracksHeaderAndRowsFitNarrowWidth(t *testing.T) {
	const width = 12
	v := View{
		Level:  TrackLevel,
		Width:  width,
		Height: 10,
		Tracks: trackRows("Long Title"),
	}
	v.Tracks[0].Artist = "Long Artist"
	v.Tracks[0].Album = "Long Album"
	v.Tracks[0].DurationMs = 60000

	lines := strings.Split(v.Render(), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected track render to include heading, header, and row; got %d lines", len(lines))
	}
	header := ansi.Strip(lines[2])
	row := ansi.Strip(lines[3])
	if got := ansi.StringWidth(header); got != width {
		t.Fatalf("track header width = %d, want %d; header=%q", got, width, header)
	}
	if got := ansi.StringWidth(row); got != width {
		t.Fatalf("track row width = %d, want %d; row=%q", got, width, row)
	}
}

func TestRenderTracksHeaderAndRowsFitTinyWidth(t *testing.T) {
	const width = 4
	v := View{
		Level:  TrackLevel,
		Width:  width,
		Height: 10,
		Tracks: trackRows("Long Title"),
	}
	v.Tracks[0].Artist = "Long Artist"
	v.Tracks[0].Album = "Long Album"
	v.Tracks[0].DurationMs = 60000

	lines := strings.Split(v.Render(), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected track render to include heading, header, and row; got %d lines", len(lines))
	}
	header := ansi.Strip(lines[2])
	row := ansi.Strip(lines[3])
	if got := ansi.StringWidth(header); got != width {
		t.Fatalf("track header width = %d, want %d; header=%q", got, width, header)
	}
	if got := ansi.StringWidth(row); got != width {
		t.Fatalf("track row width = %d, want %d; row=%q", got, width, row)
	}
}
