package playlists

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

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
