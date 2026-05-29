package library

import (
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

// TestRenderScrollOffsetBeyondTrackCount verifies that Render does not panic
// when ScrollOffset exceeds the number of tracks (e.g. after tracks are removed).
func TestRenderScrollOffsetBeyondTrackCount(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.ScrollOffset = 100

	got := v.Render()
	if !strings.Contains(got, "Artist") {
		t.Errorf("expected column header in render even with out-of-bounds scroll, got:\n%s", got)
	}
	if strings.Contains(got, "No matches") {
		t.Errorf("out-of-bounds scroll should not be reported as no filter matches, got:\n%s", got)
	}
}

func TestRenderNegativeScrollOffsetDoesNotPanic(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.ScrollOffset = -10

	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("expected render to clamp negative ScrollOffset and show first row, got:\n%s", got)
	}
}

func TestRenderWithNoVisibleRowsDoesNotClaimNoMatches(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.Height = 1

	got := v.Render()
	if !strings.Contains(got, "Artist") {
		t.Fatalf("expected column header when no data rows fit, got:\n%s", got)
	}
	if strings.Contains(got, "No matches") {
		t.Fatalf("existing tracks should not render as no matches just because no rows fit, got:\n%s", got)
	}
}

func TestRenderIgnoresStaleFilteredMatchIndices(t *testing.T) {
	v := newRenderView(sampleTracks[:1])
	v.Filter = filter.State{Text: "Alice"}
	v.FilteredMatches = []musiclib.Match{{TrackIndex: 99}}

	got := v.Render()
	if !strings.Contains(got, "No matches") {
		t.Fatalf("expected stale filtered matches to render as no matches, got:\n%s", got)
	}
	if strings.Contains(got, sampleTracks[0].Title) {
		t.Fatalf("stale filtered matches should not render an out-of-range track, got:\n%s", got)
	}
}

// TestRenderStaleScrollOffsetShowsRowsAtCursor regression-tests a render path
// where a ScrollOffset that exceeded the (now shrunk) track count caused the
// body slice to start at len(display), producing an empty body even though the
// cursor pointed at a valid row.
func TestRenderStaleScrollOffsetShowsRowsAtCursor(t *testing.T) {
	v := newRenderView(sampleTracks)
	v.Cursor = 0
	v.ScrollOffset = 999

	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Fatalf("expected stale ScrollOffset to clamp so first track renders, got:\n%s", got)
	}
}

func TestRenderBucketedShortViewportKeepsCursorVisible(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "First", Album: "A", Hash: "h1"},
		{Artist: "Alice", Title: "Second", Album: "A", Hash: "h2"},
	}
	v := newRenderView(tracks)
	v.Height = 4 // state strip + header + two body lines
	v.Cursor = 1
	v.BucketLabel = func(musiclib.Track) string { return "A" }

	got := renderPlain(v)
	if !strings.Contains(got, "Second") {
		t.Fatalf("bucket header should not push the focused row out of the viewport, got:\n%s", got)
	}
}
