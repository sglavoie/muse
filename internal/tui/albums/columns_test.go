package albums

import "testing"

func TestComputeTrackDoesNotOverAllocateNearMinimumBoundary(t *testing.T) {
	for width := 5; width <= 21; width++ {
		cols := ComputeTrack(width)
		total := cols.SelectionMarker + cols.Position + cols.Title + cols.Artist + cols.Duration + trackSepCount
		if total != width {
			t.Fatalf("ComputeTrack(%d) total=%d, want %d: %+v", width, total, width, cols)
		}
	}
}

func TestComputeTrackPreservesFixedColumnsWhenSpaceAllows(t *testing.T) {
	cols := ComputeTrack(80)
	if cols.SelectionMarker != selectionMarkerFixed {
		t.Fatalf("SelectionMarker=%d, want %d", cols.SelectionMarker, selectionMarkerFixed)
	}
	if cols.Position != trackPosFixed {
		t.Fatalf("Position=%d, want %d", cols.Position, trackPosFixed)
	}
	if cols.Duration != trackDurationFixed {
		t.Fatalf("Duration=%d, want %d", cols.Duration, trackDurationFixed)
	}
	if cols.Title <= 0 || cols.Artist <= 0 {
		t.Fatalf("expected positive title and artist columns, got %+v", cols)
	}
}

func TestComputeTrackShrinksDurationBeforePositionAtNarrowWidth(t *testing.T) {
	const width = 20
	cols := ComputeTrack(width)
	if cols.Duration != trackDurationFixed-1 {
		t.Fatalf("Duration=%d, want %d", cols.Duration, trackDurationFixed-1)
	}
	if cols.Position != trackPosFixed {
		t.Fatalf("Position=%d, want %d", cols.Position, trackPosFixed)
	}
	if cols.Title != 1 {
		t.Fatalf("Title=%d, want 1", cols.Title)
	}
	total := cols.SelectionMarker + cols.Position + cols.Title + cols.Artist + cols.Duration + trackSepCount
	if total != width {
		t.Fatalf("ComputeTrack(%d) total=%d, want %d", width, total, width)
	}
}
