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

// --- bulk-selection state ---

// TestToggleBulkSelectionToggles verifies that the first call adds the ID to
// the selected set and the second call removes it (toggle semantics).
func TestToggleBulkSelectionToggles(t *testing.T) {
	v := View{}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; !ok {
		t.Fatal("expected ID 42 to be selected after first toggle")
	}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; ok {
		t.Fatal("expected ID 42 to be deselected after second toggle")
	}
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.SelectedTrackIDs)
	}
}

// TestToggleBulkSelectionZeroIDIsNoOp verifies that toggling with id==0 does
// not add an entry to the selection set.
func TestToggleBulkSelectionZeroIDIsNoOp(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(0)
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("ToggleBulkSelection(0) must be a no-op; got %v", v.SelectedTrackIDs)
	}
}

// TestClearBulkSelectionEmptiesSet verifies that ClearBulkSelection removes
// all bulk-selected IDs.
func TestClearBulkSelectionEmptiesSet(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(1)
	v = v.ToggleBulkSelection(2)
	v = v.ClearBulkSelection()
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection after ClearBulkSelection, got %v", v.SelectedTrackIDs)
	}
}

// TestRenderTracksShowsBulletOnlyForSelected verifies that the '●' marker
// appears in the rendered output for selected rows and not for unselected ones.
func TestRenderTracksShowsBulletOnlyForSelected(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Selected Song", Artist: "A", DurationMs: 60000},
		{ID: 20, Position: 2, Title: "Unselected Song", Artist: "B", DurationMs: 60000},
	}
	v := View{
		Level:            TrackLevel,
		Width:            80,
		Tracks:           tracks,
		SelectedTrackIDs: map[int64]struct{}{10: {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Fatal("expected '●' marker in output for selected track")
	}
	// The bullet must appear on the selected track's line and not on the other.
	lines := strings.Split(out, "\n")
	var selectedLine, unselectedLine string
	for _, l := range lines {
		if strings.Contains(l, "Selected Song") {
			selectedLine = l
		}
		if strings.Contains(l, "Unselected Song") {
			unselectedLine = l
		}
	}
	if !strings.Contains(selectedLine, "●") {
		t.Errorf("selected track line must contain '●'; got: %q", selectedLine)
	}
	if strings.Contains(unselectedLine, "●") {
		t.Errorf("unselected track line must not contain '●'; got: %q", unselectedLine)
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
