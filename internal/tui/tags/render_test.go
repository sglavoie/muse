package tags

import (
	"strings"
	"testing"
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

func TestRenderTagsWithHighScrollOffsetShowsLastWindow(t *testing.T) {
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

	if !strings.Contains(out, "gamma") {
		t.Fatalf("expected render to clamp high scroll and include last tag, got:\n%s", out)
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

func TestRenderTracksWithHighScrollOffsetShowsLastWindow(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       5,
		ScrollOffset: 50,
		Tracks:       trackRows("first", "second", "third"),
	}

	out := v.Render()

	if !strings.Contains(out, "third") {
		t.Fatalf("expected render to clamp high scroll and include last track, got:\n%s", out)
	}
}
