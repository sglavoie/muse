package favorites

import "testing"

func TestVisibleRows_CategoryLevel(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 20}
	got := v.visibleRows()
	if got != 20 {
		t.Errorf("expected 20, got %d", got)
	}
}

func TestVisibleRows_ListLevel_ArtistsNoFilter(t *testing.T) {
	v := View{Level: ListLevel, Category: CategoryArtists, Height: 20}
	if got := v.visibleRows(); got != 20 {
		t.Errorf("expected 20, got %d", got)
	}
}

func TestVisibleRows_ListLevel_TracksCategory(t *testing.T) {
	v := View{Level: ListLevel, Category: CategoryTracks, Height: 20}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("expected 19, got %d", got)
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
	if got := v.visibleRows(); got != 19 {
		t.Errorf("expected 19, got %d", got)
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
