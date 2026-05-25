package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

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
			{Position: 1, Title: "Song A", Artist: "Artist X", Album: "Album P", DurationMs: 183000},
			{Position: 2, Title: "Song B", Artist: "Artist Y", Album: "Album Q", DurationMs: 245000},
		},
	}
	out := v.Render()
	for _, want := range []string{"Song A", "Artist X", "Album P", "3:03", "Song B", "Artist Y", "Album Q", "4:05"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	for _, hdr := range []string{"Title", "Artist", "Album", "Time"} {
		if !strings.Contains(out, hdr) {
			t.Errorf("expected header %q in output, got:\n%s", hdr, out)
		}
	}
}

func TestRenderTracksShowsSortArrowInActiveHeader(t *testing.T) {
	v := View{
		Level:     TrackLevel,
		Width:     80,
		SortField: "Title",
		SortDir:   "▲",
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "Song A", Artist: "Artist X", Album: "Album P", DurationMs: 183000},
		},
	}
	out := v.Render()
	if !strings.Contains(out, "Title▲") {
		t.Fatalf("expected active playlist sort header to show Title▲, got:\n%s", out)
	}
	for _, unwanted := range []string{"Artist▲", "Album▲", "Time▲"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("expected only the active playlist header to show an arrow, found %q in:\n%s", unwanted, out)
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
	tracks := []db.PlaylistTrackRow{{Position: 1, Title: "Song A", Artist: "Artist X", DurationMs: 60000}}
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

	for _, v := range []View{narrow, wide} {
		if !strings.Contains(v.Render(), "Song A") {
			t.Errorf("expected 'Song A' in render at width %d", v.Width)
		}
	}
}

func TestRenderTracksShowsSequentialPositions(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1000, Title: "Song A", Artist: "Artist X", DurationMs: 183000},
			{Position: 2000, Title: "Song B", Artist: "Artist Y", DurationMs: 245000},
			{Position: 3000, Title: "Song C", Artist: "Artist Z", DurationMs: 300000},
		},
	}
	out := v.Render()
	if strings.Contains(out, "1000") {
		t.Errorf("render should not contain raw DB position 1000, got:\n%s", out)
	}
	if strings.Contains(out, "2000") {
		t.Errorf("render should not contain raw DB position 2000, got:\n%s", out)
	}
	if strings.Contains(out, "3000") {
		t.Errorf("render should not contain raw DB position 3000, got:\n%s", out)
	}
}

func TestRenderTracksScrollOffsetBeyondCount(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		ScrollOffset: 100,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Position: 1, Title: "Song A", Artist: "X", DurationMs: 60000},
		},
	}
	out := v.Render()
	if !strings.Contains(out, "Tracks") {
		t.Errorf("expected heading in output even with out-of-bounds scroll, got:\n%s", out)
	}
	if !strings.Contains(out, "Song A") {
		t.Fatalf("expected stale scroll offset to clamp to existing track row, got:\n%s", out)
	}
}

func TestRenderTracksNegativeScrollOffsetStartsAtFirstRow(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		ScrollOffset: -5,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Position: 1, Title: "First Track", Artist: "X", DurationMs: 60000},
			{ID: 2, Position: 2, Title: "Second Track", Artist: "Y", DurationMs: 60000},
		},
	}

	out := v.Render()

	if !strings.Contains(out, "First Track") {
		t.Fatalf("expected negative scroll offset to render from first track, got:\n%s", out)
	}
}

func TestRenderTracksHeaderFitsNarrowColumns(t *testing.T) {
	const width = trackMarkerFixed + trackPosFixed + trackDurationFixed + trackSepCount + 1
	v := View{
		Level:     TrackLevel,
		Width:     width,
		SortField: "Title",
		SortDir:   "▲",
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Position: 1, Title: "Long Title", Artist: "Long Artist", Album: "Long Album", DurationMs: 60000},
		},
	}

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

func TestRenderTracksTruncatesWidePositionAndDurationCells(t *testing.T) {
	const width = 40
	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []db.PlaylistTrackRow{
			{
				ID:          1,
				DiscNumber:  123,
				TrackNumber: 456,
				Title:       "Long Title",
				Artist:      "Long Artist",
				Album:       "Long Album",
				DurationMs:  36_000_000_000,
			},
		},
	}

	lines := strings.Split(v.Render(), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected track render to include heading, header, and row; got %d lines", len(lines))
	}
	row := ansi.Strip(lines[3])
	if got := ansi.StringWidth(row); got != width {
		t.Fatalf("track row width = %d, want %d; row=%q", got, width, row)
	}
}
