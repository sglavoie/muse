package genres

import "testing"

func TestVisibleRowsGenreLevel(t *testing.T) {
	v := View{Level: GenreLevel, Height: 20}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (20-1 for strip), got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilter(t *testing.T) {
	// Strip absorbs the filter chunk so reserve stays at 1 regardless
	// of filter state.
	v := View{Level: GenreLevel, Height: 20, FilterActive: true}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19, got %d", got)
	}
}

func TestVisibleRowsArtistLevel(t *testing.T) {
	v := View{Level: ArtistLevel, Height: 20}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (20-1 for strip; breadcrumb replaces heading), got %d", got)
	}
}

func TestVisibleRowsTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 20}
	if got := v.visibleRows(); got != 18 {
		t.Errorf("want 18 (20-2 for strip + col header; breadcrumb replaces heading), got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilterText(t *testing.T) {
	// Strip absorbs the filter chunk so reserve stays at 1.
	v := View{Level: GenreLevel, Height: 20, FilterText: "roc"}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19, got %d", got)
	}
}
