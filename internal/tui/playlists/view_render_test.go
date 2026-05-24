package playlists

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/text"
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

	for _, v := range []View{narrow, wide} {
		if !strings.Contains(v.Render(), "Song A") {
			t.Errorf("expected 'Song A' in render at width %d", v.Width)
		}
	}
}

func TestRenderListEmpty(t *testing.T) {
	v := View{}
	out := v.Render()
	if !strings.Contains(out, "No playlists yet") {
		t.Fatalf("expected 'No playlists yet' title in output, got:\n%s", out)
	}
	for _, want := range []string{"new", "import m3u", "help"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty playlists card missing CTA %q; got:\n%s", want, out)
		}
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

func TestRenderListShowsSortIndicatorWhenSorted(t *testing.T) {
	v := View{
		Width:     40,
		SortField: "Name",
		SortDir:   "▼",
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Chill", TrackCount: 5},
		},
	}
	out := v.Render()
	if !strings.Contains(out, "Sort: Name ▼") {
		t.Fatalf("expected playlist list sort indicator in output, got:\n%s", out)
	}
}

func TestRenderListPendingSmartCountFitsWidth(t *testing.T) {
	const width = 20
	v := View{
		Width: width,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Long Smart Playlist", TrackCount: -1, IsSmart: true},
		},
	}

	line := ansi.Strip(v.Render())
	if got := ansi.StringWidth(line); got != width {
		t.Fatalf("pending smart playlist row width = %d, want %d; line=%q", got, width, line)
	}
	if !strings.Contains(line, "(smart)") {
		t.Fatalf("expected full smart placeholder to fit, got %q", line)
	}
}

func TestRenderListLargeCountFitsWidth(t *testing.T) {
	const width = 20
	v := View{
		Width: width,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Huge", TrackCount: 123456789},
		},
	}

	line := ansi.Strip(v.Render())
	if got := ansi.StringWidth(line); got != width {
		t.Fatalf("large count playlist row width = %d, want %d; line=%q", got, width, line)
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

func TestRenderListScrollOffsetBeyondCount(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Width:        40,
		ScrollOffset: 100,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Only"},
		},
	}
	out := v.Render()

	if !strings.Contains(out, "Only") {
		t.Fatalf("expected stale scroll offset to clamp to existing playlist row, got:\n%s", out)
	}
}

func TestRenderListNegativeScrollOffsetStartsAtFirstRow(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Width:        40,
		ScrollOffset: -5,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "First"},
			{ID: 2, Name: "Second"},
		},
	}

	out := v.Render()

	if !strings.Contains(out, "First") {
		t.Fatalf("expected negative scroll offset to render from first row, got:\n%s", out)
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

// trackColumnWidthsFromRender renders the playlists track view at the given
// width, strips ANSI sequences from the first track row, and returns the
// visible rune widths of the Title and Artist columns.
func trackColumnWidthsFromRender(t *testing.T, width int) (titleW, artistW int) {
	t.Helper()
	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "Long Title That Fills The Column", Artist: "Long Artist Name", DurationMs: 183000},
		},
	}
	rendered := v.Render()

	lines := strings.Split(rendered, "\n")
	if len(lines) < 4 {
		t.Fatalf("trackColumnWidthsFromRender: expected ≥4 lines, got %d", len(lines))
	}

	trackRow := ansi.Strip(lines[3])
	runes := []rune(trackRow)
	if len(runes) != width {
		t.Fatalf("trackColumnWidthsFromRender: stripped row has %d runes, want %d", len(runes), width)
	}

	cols := ComputeTrack(width)
	titleStart := trackMarkerFixed + trackPosFixed + 1
	titleEnd := titleStart + cols.Title
	artistStart := titleEnd + 1
	artistEnd := artistStart + cols.Artist

	if artistEnd > len(runes) {
		t.Fatalf("trackColumnWidthsFromRender: artist column extends beyond row: artistEnd=%d len=%d", artistEnd, len(runes))
	}

	return len(runes[titleStart:titleEnd]), len(runes[artistStart:artistEnd])
}

func assertTrackRatioWithinTolerance(t *testing.T, colName string, colWidth, flexWidth int, wantPct float64) {
	t.Helper()
	gotPct := float64(colWidth) / float64(flexWidth) * 100
	if math.Abs(gotPct-wantPct) > 3.0 {
		t.Errorf("column %s: ratio %.2f%% (width=%d/flex=%d) exceeds ±3%% tolerance from expected %.2f%%",
			colName, gotPct, colWidth, flexWidth, wantPct)
	}
}

func TestTitleColumnRatioAtWidth80(t *testing.T) {
	const width = 80
	flex := width - trackMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	titleW, _ := trackColumnWidthsFromRender(t, width)
	wantPct := float64(trackTitleRatio) / float64(trackTotalRatio) * 100
	assertTrackRatioWithinTolerance(t, "Title", titleW, flex, wantPct)
}

func TestArtistColumnRatioAtWidth80(t *testing.T) {
	const width = 80
	flex := width - trackMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	_, artistW := trackColumnWidthsFromRender(t, width)
	wantPct := float64(trackArtistRatio) / float64(trackTotalRatio) * 100
	assertTrackRatioWithinTolerance(t, "Artist", artistW, flex, wantPct)
}

func TestFormatDurationHours(t *testing.T) {
	got := text.FormatDurationMs(3661000)
	if got != "1:01:01" {
		t.Errorf("FormatDurationMs hours: expected 1:01:01, got %s", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := text.FormatDurationMs(0)
	if got != "--:--" {
		t.Errorf("FormatDurationMs 0: expected --:--, got %s", got)
	}
}

func TestFormatDurationNegative(t *testing.T) {
	got := text.FormatDurationMs(-1000)
	if got != "--:--" {
		t.Errorf("FormatDurationMs negative: expected --:--, got %s", got)
	}
}

func TestTruncateZeroMaxLen(t *testing.T) {
	got := text.Truncate("hello", 0)
	if got != "" {
		t.Errorf("Truncate maxLen=0: expected empty, got %q", got)
	}
}

func TestTruncateNegativeMaxLen(t *testing.T) {
	got := text.Truncate("hello", -1)
	if got != "" {
		t.Errorf("Truncate maxLen=-1: expected empty, got %q", got)
	}
}

func TestTruncateMaxLenOne(t *testing.T) {
	got := text.Truncate("hello", 1)
	if got != "…" {
		t.Errorf("Truncate maxLen=1: expected '…', got %q", got)
	}
}

func TestTruncateNoTruncationNeeded(t *testing.T) {
	got := text.Truncate("hi", 5)
	if got != "hi" {
		t.Errorf("Truncate no-op: expected 'hi', got %q", got)
	}
}
