package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestSortPlaylists(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock", TrackCount: 10},
		{ID: 2, Name: "Alpha", TrackCount: 5},
		{ID: 3, Name: "Jazz", TrackCount: 20},
	}

	t.Run("sort by name asc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortName, SortAsc)
		if len(sorted) != 3 {
			t.Fatalf("expected 3 playlists, got %d", len(sorted))
		}
		if sorted[0].Name != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Name)
		}
		if sorted[1].Name != "Jazz" {
			t.Errorf("second should be Jazz, got %q", sorted[1].Name)
		}
		if sorted[2].Name != "Rock" {
			t.Errorf("third should be Rock, got %q", sorted[2].Name)
		}
	})

	t.Run("sort by name desc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortName, SortDesc)
		if sorted[0].Name != "Rock" {
			t.Errorf("first should be Rock, got %q", sorted[0].Name)
		}
		if sorted[2].Name != "Alpha" {
			t.Errorf("last should be Alpha, got %q", sorted[2].Name)
		}
	})

	t.Run("sort by track count asc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortTrackCount, SortAsc)
		if sorted[0].TrackCount != 5 {
			t.Errorf("first should have 5 tracks, got %d", sorted[0].TrackCount)
		}
		if sorted[2].TrackCount != 20 {
			t.Errorf("last should have 20 tracks, got %d", sorted[2].TrackCount)
		}
	})

	t.Run("sort by track count desc", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortTrackCount, SortDesc)
		if sorted[0].TrackCount != 20 {
			t.Errorf("first should have 20 tracks, got %d", sorted[0].TrackCount)
		}
		if sorted[2].TrackCount != 5 {
			t.Errorf("last should have 5 tracks, got %d", sorted[2].TrackCount)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		sorted := SortPlaylists(playlists, SortDefault, SortAsc)
		if sorted != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("original not modified", func(t *testing.T) {
		_ = SortPlaylists(playlists, SortName, SortAsc)
		if playlists[0].Name != "Rock" {
			t.Error("original slice was modified")
		}
	})
}

func TestSortPlaylistTrackIndices(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Title: "Zulu", Artist: "Charlie", Album: "Gamma", DurationMs: 3000},
		{ID: 2, Title: "Alpha", Artist: "Bravo", Album: "Beta", DurationMs: 1000},
		{ID: 3, Title: "Mike", Artist: "Alpha", Album: "Alpha", DurationMs: 2000},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortTitle, SortAsc)
		if order == nil {
			t.Fatal("expected non-nil order")
		}
		if tracks[order[0]].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", tracks[order[0]].Title)
		}
		if tracks[order[1]].Title != "Mike" {
			t.Errorf("second should be Mike, got %q", tracks[order[1]].Title)
		}
		if tracks[order[2]].Title != "Zulu" {
			t.Errorf("third should be Zulu, got %q", tracks[order[2]].Title)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortArtist, SortDesc)
		if tracks[order[0]].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", tracks[order[0]].Artist)
		}
		if tracks[order[2]].Artist != "Alpha" {
			t.Errorf("last should be Alpha, got %q", tracks[order[2]].Artist)
		}
	})

	t.Run("sort by duration desc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortDuration, SortDesc)
		if tracks[order[0]].DurationMs != 3000 {
			t.Errorf("first should have 3000ms, got %d", tracks[order[0]].DurationMs)
		}
		if tracks[order[2]].DurationMs != 1000 {
			t.Errorf("last should have 1000ms, got %d", tracks[order[2]].DurationMs)
		}
	})

	t.Run("sort by album asc", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortAlbum, SortAsc)
		if tracks[order[0]].Album != "Alpha" {
			t.Errorf("first should be Alpha, got %q", tracks[order[0]].Album)
		}
		if tracks[order[1]].Album != "Beta" {
			t.Errorf("second should be Beta, got %q", tracks[order[1]].Album)
		}
		if tracks[order[2]].Album != "Gamma" {
			t.Errorf("third should be Gamma, got %q", tracks[order[2]].Album)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortDefault, SortAsc)
		if order != nil {
			t.Error("SortDefault should return nil")
		}
	})
}

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
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}
	v = v.recomputePlaylistSort()
	if v.sortedPlaylists[0].Name != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.sortedPlaylists[0].Name)
	}

	tracks := []db.PlaylistTrackRow{{ID: 1, Title: "T1"}}
	v2 := v.openTracks(tracks)
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}
	if v2.sortedPlaylists != nil {
		t.Error("sortedPlaylists should be nil after openTracks")
	}

	v2.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	v2.sortedTrackOrder = []int{0}
	v3 := v2.backToList()
	if v3.sortState.Field != SortDefault {
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
			sortState: SortState{Field: SortName, Dir: SortAsc},
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
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
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
			sortState: SortState{Field: SortDefault},
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
