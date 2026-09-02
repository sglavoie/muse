package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestPlaylistsTrackIndex(t *testing.T) {
	v := PlaylistsView{
		tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "A"},
			{ID: 2, Title: "B"},
			{ID: 3, Title: "C"},
		},
	}

	t.Run("no sort order returns identity", func(t *testing.T) {
		if v.trackIndex(0) != 0 {
			t.Error("expected 0")
		}
	})

	t.Run("with sort order translates", func(t *testing.T) {
		v.sortedTrackOrder = []int{2, 0, 1}
		if v.trackIndex(0) != 2 {
			t.Errorf("display 0 should map to track 2, got %d", v.trackIndex(0))
		}
	})
}

func TestPlaylistsSortResetOnDrillDown(t *testing.T) {
	v := PlaylistsView{
		playlists: []db.PlaylistRow{
			{ID: 1, Name: "Rock", TrackCount: 10},
			{ID: 2, Name: "Alpha", TrackCount: 5},
		},
		sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}
	v = v.recomputePlaylistSort()
	if v.sortedPlaylists[0].Name != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.sortedPlaylists[0].Name)
	}

	tracks := []db.PlaylistTrackRow{{ID: 1, Title: "T1"}}
	v2 := v.openTracks(tracks)
	if v2.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openTracks")
	}
	if v2.sortedPlaylists != nil {
		t.Error("sortedPlaylists should be nil after openTracks")
	}

	v2.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	v2.sortedTrackOrder = []int{0}
	v3 := v2.backToList()
	if v3.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on backToList")
	}
	if v3.sortedTrackOrder != nil {
		t.Error("sortedTrackOrder should be nil after backToList")
	}
}

func TestPlaylistsFindTrackDisplayPosition(t *testing.T) {
	v := PlaylistsView{
		tracks: []db.PlaylistTrackRow{{ID: 1}, {ID: 2}, {ID: 3}},
	}

	if v.findTrackDisplayPosition(1) != 1 {
		t.Error("expected identity mapping without sort order")
	}

	v.sortedTrackOrder = []int{2, 0, 1}
	if got := v.findTrackDisplayPosition(0); got != 1 {
		t.Errorf("track 0 should be at display 1, got %d", got)
	}
	if got := v.findTrackDisplayPosition(2); got != 0 {
		t.Errorf("track 2 should be at display 0, got %d", got)
	}
}

func TestPlaylistsRecomputeSort(t *testing.T) {
	t.Run("list level", func(t *testing.T) {
		v := PlaylistsView{
			level: pltui.ListLevel,
			playlists: []db.PlaylistRow{
				{ID: 1, Name: "Zulu", TrackCount: 5},
				{ID: 2, Name: "Alpha", TrackCount: 10},
			},
			sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
		}
		v = v.recomputePlaylistSort()
		if v.sortedPlaylists == nil {
			t.Fatal("expected non-nil sortedPlaylists")
		}
		if v.sortedPlaylists[0].Name != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.sortedPlaylists[0].Name)
		}
		if v.sortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil at list level")
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := PlaylistsView{
			level: pltui.TrackLevel,
			tracks: []db.PlaylistTrackRow{
				{ID: 1, Title: "Zulu", DurationMs: 3000},
				{ID: 2, Title: "Alpha", DurationMs: 1000},
			},
			sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		}
		v = v.recomputePlaylistSort()
		if v.sortedTrackOrder == nil {
			t.Fatal("expected non-nil sortedTrackOrder")
		}
		if v.tracks[v.sortedTrackOrder[0]].Title != "Alpha" {
			t.Error("first display track should be Alpha")
		}
		if v.sortedPlaylists != nil {
			t.Error("sortedPlaylists should be nil at track level")
		}
	})

	t.Run("default sort clears caches", func(t *testing.T) {
		v := PlaylistsView{
			level:     pltui.ListLevel,
			playlists: []db.PlaylistRow{{ID: 1, Name: "A"}},
			sortState: sorting.SortState{Field: sorting.SortDefault},
		}
		v = v.recomputePlaylistSort()
		if v.sortedPlaylists != nil {
			t.Error("sortedPlaylists should be nil for SortDefault")
		}
	})
}

func TestPlaylistsEffectivePlaylists(t *testing.T) {
	orig := []db.PlaylistRow{{ID: 1, Name: "Zulu"}, {ID: 2, Name: "Alpha"}}
	sorted := []db.PlaylistRow{{ID: 2, Name: "Alpha"}, {ID: 1, Name: "Zulu"}}

	v := PlaylistsView{playlists: orig}
	if v.effectivePlaylists()[0].Name != "Zulu" {
		t.Error("without sort, should return original")
	}

	v.sortedPlaylists = sorted
	if v.effectivePlaylists()[0].Name != "Alpha" {
		t.Error("with sort, should return sorted")
	}
}

func TestPlaylistsTrackIndexBoundsCheck(t *testing.T) {
	v := PlaylistsView{
		tracks:           []db.PlaylistTrackRow{{ID: 1}, {ID: 2}},
		sortedTrackOrder: []int{1, 0},
	}

	if got := v.trackIndex(-1); got != -1 {
		t.Errorf("negative index: got %d, want -1", got)
	}
	if got := v.trackIndex(5); got != 5 {
		t.Errorf("over-bounds index: got %d, want 5", got)
	}
}
