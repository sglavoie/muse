package tags

import "testing"

func TestComputeTrack_DoesNotOverAllocateNearMinimumBoundary(t *testing.T) {
	for width := 43; width <= 49; width++ {
		cols := ComputeTrack(width)
		total := cols.SelectionMarker + cols.Title + cols.Artist + cols.Album + cols.Duration + 2
		if total != width {
			t.Fatalf("ComputeTrack(%d) total=%d, want %d", width, total, width)
		}
	}
}

func TestComputeTrack_PreservesMinimumsWhenSpaceAllows(t *testing.T) {
	cols := ComputeTrack(80)
	if cols.Title < 10 {
		t.Fatalf("Title=%d, want >=%d", cols.Title, 10)
	}
	if cols.Artist < 10 {
		t.Fatalf("Artist=%d, want >=%d", cols.Artist, 10)
	}
	if cols.Album < 8 {
		t.Fatalf("Album=%d, want >=%d", cols.Album, 8)
	}
}

func TestComputeTrack_NarrowWidthStaysNonNegativeAndFits(t *testing.T) {
	cols := ComputeTrack(20)
	if cols.Title < 0 || cols.Artist < 0 || cols.Album < 0 {
		t.Fatalf("negative flex widths: %+v", cols)
	}
	total := cols.SelectionMarker + cols.Title + cols.Artist + cols.Album + cols.Duration + trackSepCount
	if total != 20 {
		t.Fatalf("ComputeTrack(20) total=%d, want 20", total)
	}
}

func TestComputeTrack_ShrinksDurationWhenFixedBudgetWouldOverflow(t *testing.T) {
	const width = 12
	cols := ComputeTrack(width)
	if cols.Duration != 7 {
		t.Fatalf("Duration=%d, want 7", cols.Duration)
	}
	total := cols.SelectionMarker + cols.Title + cols.Artist + cols.Album + cols.Duration + trackSepCount
	if total != width {
		t.Fatalf("ComputeTrack(%d) total=%d, want %d", width, total, width)
	}
}
