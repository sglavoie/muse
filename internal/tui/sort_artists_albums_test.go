package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestArtistsViewSortReset(t *testing.T) {
	v := ArtistsView{
		artists:   []string{"Charlie", "Alpha", "Bravo"},
		sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	t.Run("openAlbums resets sort", func(t *testing.T) {
		v2 := v.openAlbums([]string{"Album1"})
		if v2.sortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openAlbums")
		}
	})

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.openTracks([]library.Track{{Title: "T1"}})
		if v2.sortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.level = 1
		v2.albums = []string{"Album1"}
		v2.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
		v3 := v2.back()
		if v3.sortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestAlbumsViewSortReset(t *testing.T) {
	v := AlbumsView{
		albums:    []string{"Zulu", "Alpha"},
		sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.openTracks([]library.Track{{Title: "T1"}})
		if v2.sortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.level = 1
		v2.tracks = []library.Track{{Title: "T1"}}
		v2.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
		v3 := v2.back()
		if v3.sortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestArtistsRecomputeSort(t *testing.T) {
	t.Run("sort artists by name asc", func(t *testing.T) {
		v := ArtistsView{
			artists:   []string{"Charlie", "Alpha", "Bravo"},
			sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
		}
		v = v.recomputeArtistsSort()
		if v.artists[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.artists[0])
		}
		if v.artists[2] != "Charlie" {
			t.Errorf("last should be Charlie, got %q", v.artists[2])
		}
	})

	t.Run("sort albums by name desc", func(t *testing.T) {
		v := ArtistsView{
			level:     1,
			albums:    []string{"Alpha", "Charlie", "Bravo"},
			sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc},
		}
		v = v.recomputeArtistsSort()
		if v.albums[0] != "Charlie" {
			t.Errorf("first should be Charlie, got %q", v.albums[0])
		}
	})

	t.Run("sort tracks by title asc", func(t *testing.T) {
		v := ArtistsView{
			level: 2,
			tracks: []library.Track{
				{Title: "Zulu", Duration: 300},
				{Title: "Alpha", Duration: 100},
			},
			sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		}
		v = v.recomputeArtistsSort()
		if v.tracks[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.tracks[0].Title)
		}
	})

	t.Run("sort tracks by duration desc", func(t *testing.T) {
		v := ArtistsView{
			level: 2,
			tracks: []library.Track{
				{Title: "A", Duration: 100},
				{Title: "B", Duration: 300},
			},
			sortState: sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortDesc},
		}
		v = v.recomputeArtistsSort()
		if v.tracks[0].Duration != 300 {
			t.Errorf("first should have duration 300, got %f", v.tracks[0].Duration)
		}
	})

	t.Run("default is no-op", func(t *testing.T) {
		v := ArtistsView{
			artists:   []string{"Charlie", "Alpha"},
			sortState: sorting.SortState{Field: sorting.SortDefault},
		}
		v = v.recomputeArtistsSort()
		if v.artists[0] != "Charlie" {
			t.Error("default should not reorder")
		}
	})
}

func TestAlbumsRecomputeSort(t *testing.T) {
	t.Run("sort albums by name asc", func(t *testing.T) {
		v := AlbumsView{
			albums:    []string{"Zulu", "Alpha", "Bravo"},
			sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
		}
		v = v.recomputeAlbumsSort()
		if v.albums[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.albums[0])
		}
	})

	t.Run("sort tracks by title desc", func(t *testing.T) {
		v := AlbumsView{
			level: 1,
			tracks: []library.Track{
				{Title: "Alpha", Duration: 100},
				{Title: "Zulu", Duration: 300},
			},
			sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc},
		}
		v = v.recomputeAlbumsSort()
		if v.tracks[0].Title != "Zulu" {
			t.Errorf("first should be Zulu, got %q", v.tracks[0].Title)
		}
	})
}

func TestSortResetOnArtistsDrillDown(t *testing.T) {
	v := ArtistsView{
		artists:   []string{"Charlie", "Alpha", "Bravo"},
		sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	v = v.recomputeArtistsSort()
	if v.artists[0] != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.artists[0])
	}

	v2 := v.openAlbums([]string{"Album1", "Album2"})
	if v2.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openAlbums")
	}

	v2.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	v3 := v2.openTracks([]library.Track{{Title: "T1"}})
	if v3.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v3.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	v4 := v3.back()
	if v4.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on back")
	}
}

func TestSortResetOnAlbumsDrillDown(t *testing.T) {
	v := AlbumsView{
		albums:    []string{"Zulu", "Alpha"},
		sortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}
	v = v.recomputeAlbumsSort()

	v2 := v.openTracks([]library.Track{{Title: "T1"}})
	if v2.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v2.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	v3 := v2.back()
	if v3.sortState.Field != sorting.SortDefault {
		t.Error("sort should reset on back")
	}
}

func TestArtistsSortFieldsForLevel(t *testing.T) {
	tests := []struct {
		level arttui.Level
		want  []sorting.SortField
	}{
		{arttui.ArtistLevel, sorting.ArtistArtistSortFields},
		{arttui.AlbumLevel, sorting.ArtistAlbumSortFields},
		{arttui.TrackLevel, sorting.ArtistTrackSortFields},
	}
	for _, tt := range tests {
		got := artistsSortFieldsForLevel(tt.level)
		if len(got) != len(tt.want) {
			t.Errorf("level %d: len=%d, want %d", tt.level, len(got), len(tt.want))
			continue
		}
		for i, f := range got {
			if f != tt.want[i] {
				t.Errorf("level %d, idx %d: got %d, want %d", tt.level, i, f, tt.want[i])
			}
		}
	}
}

func TestAlbumsSortFieldsForLevel(t *testing.T) {
	tests := []struct {
		level albtui.Level
		want  []sorting.SortField
	}{
		{albtui.AlbumLevel, sorting.AlbumAlbumSortFields},
		{albtui.TrackLevel, sorting.AlbumTrackSortFields},
	}
	for _, tt := range tests {
		got := albumsSortFieldsForLevel(tt.level)
		if len(got) != len(tt.want) {
			t.Errorf("level %d: len=%d, want %d", tt.level, len(got), len(tt.want))
			continue
		}
		for i, f := range got {
			if f != tt.want[i] {
				t.Errorf("level %d, idx %d: got %d, want %d", tt.level, i, f, tt.want[i])
			}
		}
	}
}

func TestArtistsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    ArtistsView
		want int
	}{
		{"artist level no filter no sort", ArtistsView{level: arttui.ArtistLevel}, 1},
		{"artist level with filter (strip absorbs)", ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}}, 1},
		{"artist level with sort (strip absorbs)", ArtistsView{level: arttui.ArtistLevel, sortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"artist level with filter and sort (strip absorbs)", ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}, sortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"album level no sort", ArtistsView{level: arttui.AlbumLevel}, 2},
		{"album level with sort (strip absorbs)", ArtistsView{level: arttui.AlbumLevel, sortState: sorting.SortState{Field: sorting.SortName}}, 2},
		{"track level no sort", ArtistsView{level: arttui.TrackLevel}, 3},
		{"track level with sort (strip absorbs)", ArtistsView{level: arttui.TrackLevel, sortState: sorting.SortState{Field: sorting.SortTitle}}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artistsVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("artistsVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAlbumsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    AlbumsView
		want int
	}{
		{"album level no filter no sort", AlbumsView{level: albtui.AlbumLevel}, 1},
		{"album level with filter (strip absorbs)", AlbumsView{level: albtui.AlbumLevel, filter: filter.State{Active: true}}, 1},
		{"album level with sort (strip absorbs)", AlbumsView{level: albtui.AlbumLevel, sortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"track level no sort", AlbumsView{level: albtui.TrackLevel}, 3},
		{"track level with sort (strip absorbs)", AlbumsView{level: albtui.TrackLevel, sortState: sorting.SortState{Field: sorting.SortTitle}}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := albumsVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("albumsVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}
