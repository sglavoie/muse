package favorites

import "testing"

func TestVisibleRows_CategoryLevel(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 20}
	got := v.visibleRows()
	if got != 19 {
		t.Errorf("expected 19, got %d", got)
	}
}

func TestVisibleRows_ListLevel_ArtistsNoFilter(t *testing.T) {
	v := View{Level: ListLevel, Category: CategoryArtists, Height: 20}
	if got := v.visibleRows(); got != 18 {
		t.Errorf("expected 18, got %d", got)
	}
}

func TestVisibleRows_ListLevel_TracksCategory(t *testing.T) {
	v := View{Level: ListLevel, Category: CategoryTracks, Height: 20}
	if got := v.visibleRows(); got != 17 {
		t.Errorf("expected 17, got %d", got)
	}
}

func TestVisibleRows_TrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 20}
	if got := v.visibleRows(); got != 17 {
		t.Errorf("expected 17, got %d", got)
	}
}

func TestVisibleRows_WithSortIndicator(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 20, SortIndicator: "Sort: Name ▲"}
	if got := v.visibleRows(); got != 18 {
		t.Errorf("expected 18, got %d", got)
	}
}

func TestVisibleRows_ZeroHeight(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 0}
	got := v.visibleRows()
	if got != 50 {
		t.Errorf("expected default 50, got %d", got)
	}
}

func TestComputeTrack_NarrowWidth(t *testing.T) {
	cols := ComputeTrack(10)
	if cols.Title != 1 {
		t.Errorf("expected Title=1 for narrow width, got %d", cols.Title)
	}
	if cols.Artist != 0 {
		t.Errorf("expected Artist=0 for narrow width, got %d", cols.Artist)
	}
}

func TestComputeTrack_DoesNotOverAllocateWidthAtBoundary(t *testing.T) {
	cols := ComputeTrack(29)
	total := cols.Marker + cols.Position + cols.Title + cols.Artist + cols.Album +
		cols.Duration + cols.PlayCount + trackSepCount
	if total != 29 {
		t.Fatalf("columns should fit boundary width 29, got %d", total)
	}
	if cols.Title < 1 {
		t.Fatalf("expected to preserve at least one title column at width 29, got %d", cols.Title)
	}
}

func TestComputeTrack_ShrinksDataColumnsWhenOnlyScaffoldFits(t *testing.T) {
	cols := ComputeTrack(trackMarkerFixed + trackSepCount)
	if cols.Position != 0 || cols.Title != 0 || cols.Artist != 0 || cols.Album != 0 ||
		cols.Duration != 0 || cols.PlayCount != 0 {
		t.Fatalf("expected zero-width data columns, got %+v", cols)
	}
	total := cols.Marker + cols.Position + cols.Title + cols.Artist + cols.Album +
		cols.Duration + cols.PlayCount + trackSepCount
	if total != trackMarkerFixed+trackSepCount {
		t.Fatalf("columns should fit scaffold-only width, got %d", total)
	}
}

func TestComputeTrack_NormalWidth(t *testing.T) {
	cols := ComputeTrack(120)
	if cols.Title <= 0 || cols.Artist <= 0 || cols.Album <= 0 {
		t.Errorf("expected positive flex columns, got title=%d artist=%d album=%d",
			cols.Title, cols.Artist, cols.Album)
	}
	total := cols.Marker + cols.Position + cols.Title + cols.Artist + cols.Album +
		cols.Duration + cols.PlayCount + trackSepCount
	if total != 120 {
		t.Errorf("columns should sum to total width 120, got %d", total)
	}
}
