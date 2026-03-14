package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- track-level rendering ---

func TestRenderTracksEmpty(t *testing.T) {
	v := View{Level: TrackLevel}
	out := v.Render()
	if !strings.Contains(out, "no tracks") {
		t.Fatalf("expected '(no tracks)' in output, got:\n%s", out)
	}
}

func TestRenderTracksShowsColumns(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "Song A", Artist: "Artist X", DurationMs: 183000},
			{Position: 2, Title: "Song B", Artist: "Artist Y", DurationMs: 245000},
		},
	}
	out := v.Render()
	for _, want := range []string{"Song A", "Artist X", "3:03", "Song B", "Artist Y", "4:05"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderTracksAscendingPositionOrder(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "First"},
			{Position: 2, Title: "Second"},
			{Position: 3, Title: "Third"},
		},
	}
	out := v.Render()
	posFirst := strings.Index(out, "First")
	posSecond := strings.Index(out, "Second")
	posThird := strings.Index(out, "Third")
	if !(posFirst < posSecond && posSecond < posThird) {
		t.Errorf("tracks not in ascending position order:\n%s", out)
	}
}

func TestRenderTracksColumnResizeOnWidthChange(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Position: 1, Title: "Song A", Artist: "Artist X", DurationMs: 60000},
	}
	narrow := View{Level: TrackLevel, Width: 40, Tracks: tracks}
	wide := View{Level: TrackLevel, Width: 120, Tracks: tracks}

	colsNarrow := ComputeTrack(40)
	colsWide := ComputeTrack(120)

	if colsNarrow.Title >= colsWide.Title {
		t.Errorf("expected title column to grow with width: narrow=%d wide=%d", colsNarrow.Title, colsWide.Title)
	}
	if colsNarrow.Artist >= colsWide.Artist {
		t.Errorf("expected artist column to grow with width: narrow=%d wide=%d", colsNarrow.Artist, colsWide.Artist)
	}

	// Both renders should contain the track
	for _, v := range []View{narrow, wide} {
		if !strings.Contains(v.Render(), "Song A") {
			t.Errorf("expected 'Song A' in render at width %d", v.Width)
		}
	}
}

func TestRenderListEmpty(t *testing.T) {
	v := View{}
	out := v.Render()
	if !strings.Contains(out, "no playlists") {
		t.Fatalf("expected '(no playlists)' in output, got:\n%s", out)
	}
}

func TestRenderListShowsNameAndCount(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Chill", TrackCount: 5},
			{ID: 2, Name: "Workout", TrackCount: 12},
		},
		Width: 40,
	}
	out := v.Render()
	if !strings.Contains(out, "Chill") {
		t.Errorf("expected 'Chill' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(5)") {
		t.Errorf("expected '(5)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Workout") {
		t.Errorf("expected 'Workout' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(12)") {
		t.Errorf("expected '(12)' in output, got:\n%s", out)
	}
}
