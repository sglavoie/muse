package genres

import "testing"

func TestComputeTrackDoesNotOverAllocateNearMinimumBoundary(t *testing.T) {
	for width := 2; width <= 20; width++ {
		cols := ComputeTrack(width)
		total := cols.SelectionMarker + cols.Position + cols.Title + cols.Duration + trackSepCount
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
	if cols.Title <= 0 {
		t.Fatalf("expected positive title column, got %+v", cols)
	}
}
