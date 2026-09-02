package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestArtistsViewSortReset(t *testing.T) {
	v := browse.ArtistsView{
		Artists:   []string{"Charlie", "Alpha", "Bravo"},
		SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	t.Run("openAlbums resets sort", func(t *testing.T) {
		v2 := v.OpenAlbums([]string{"Album1"})
		if v2.SortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openAlbums")
		}
	})

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.OpenTracks([]library.Track{{Title: "T1"}})
		if v2.SortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.Level = 1
		v2.Albums = []string{"Album1"}
		v2.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
		v3 := v2.Back()
		if v3.SortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestAlbumsViewSortReset(t *testing.T) {
	v := browse.AlbumsView{
		Albums:    []string{"Zulu", "Alpha"},
		SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.OpenTracks([]library.Track{{Title: "T1"}})
		if v2.SortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.Level = 1
		v2.Tracks = []library.Track{{Title: "T1"}}
		v2.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
		v3 := v2.Back()
		if v3.SortState.Field != sorting.SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestArtistsRecomputeSort(t *testing.T) {
	t.Run("sort artists by name asc", func(t *testing.T) {
		v := browse.ArtistsView{
			Artists:   []string{"Charlie", "Alpha", "Bravo"},
			SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
		}
		v = v.RecomputeArtistsSort()
		if v.Artists[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.Artists[0])
		}
		if v.Artists[2] != "Charlie" {
			t.Errorf("last should be Charlie, got %q", v.Artists[2])
		}
	})

	t.Run("sort albums by name desc", func(t *testing.T) {
		v := browse.ArtistsView{
			Level:     1,
			Albums:    []string{"Alpha", "Charlie", "Bravo"},
			SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc},
		}
		v = v.RecomputeArtistsSort()
		if v.Albums[0] != "Charlie" {
			t.Errorf("first should be Charlie, got %q", v.Albums[0])
		}
	})

	t.Run("sort tracks by title asc", func(t *testing.T) {
		v := browse.ArtistsView{
			Level: 2,
			Tracks: []library.Track{
				{Title: "Zulu", Duration: 300},
				{Title: "Alpha", Duration: 100},
			},
			SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		}
		v = v.RecomputeArtistsSort()
		if v.Tracks[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.Tracks[0].Title)
		}
	})

	t.Run("sort tracks by duration desc", func(t *testing.T) {
		v := browse.ArtistsView{
			Level: 2,
			Tracks: []library.Track{
				{Title: "A", Duration: 100},
				{Title: "B", Duration: 300},
			},
			SortState: sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortDesc},
		}
		v = v.RecomputeArtistsSort()
		if v.Tracks[0].Duration != 300 {
			t.Errorf("first should have duration 300, got %f", v.Tracks[0].Duration)
		}
	})

	t.Run("default is no-op", func(t *testing.T) {
		v := browse.ArtistsView{
			Artists:   []string{"Charlie", "Alpha"},
			SortState: sorting.SortState{Field: sorting.SortDefault},
		}
		v = v.RecomputeArtistsSort()
		if v.Artists[0] != "Charlie" {
			t.Error("default should not reorder")
		}
	})
}

func TestAlbumsRecomputeSort(t *testing.T) {
	t.Run("sort albums by name asc", func(t *testing.T) {
		v := browse.AlbumsView{
			Albums:    []string{"Zulu", "Alpha", "Bravo"},
			SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
		}
		v = v.RecomputeAlbumsSort()
		if v.Albums[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", v.Albums[0])
		}
	})

	t.Run("sort tracks by title desc", func(t *testing.T) {
		v := browse.AlbumsView{
			Level: 1,
			Tracks: []library.Track{
				{Title: "Alpha", Duration: 100},
				{Title: "Zulu", Duration: 300},
			},
			SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc},
		}
		v = v.RecomputeAlbumsSort()
		if v.Tracks[0].Title != "Zulu" {
			t.Errorf("first should be Zulu, got %q", v.Tracks[0].Title)
		}
	})
}

func TestSortResetOnArtistsDrillDown(t *testing.T) {
	v := browse.ArtistsView{
		Artists:   []string{"Charlie", "Alpha", "Bravo"},
		SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}

	v = v.RecomputeArtistsSort()
	if v.Artists[0] != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.Artists[0])
	}

	v2 := v.OpenAlbums([]string{"Album1", "Album2"})
	if v2.SortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openAlbums")
	}

	v2.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	v3 := v2.OpenTracks([]library.Track{{Title: "T1"}})
	if v3.SortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v3.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	v4 := v3.Back()
	if v4.SortState.Field != sorting.SortDefault {
		t.Error("sort should reset on back")
	}
}

func TestSortResetOnAlbumsDrillDown(t *testing.T) {
	v := browse.AlbumsView{
		Albums:    []string{"Zulu", "Alpha"},
		SortState: sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc},
	}
	v = v.RecomputeAlbumsSort()

	v2 := v.OpenTracks([]library.Track{{Title: "T1"}})
	if v2.SortState.Field != sorting.SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v2.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	v3 := v2.Back()
	if v3.SortState.Field != sorting.SortDefault {
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
		v    browse.ArtistsView
		want int
	}{
		{"artist level no filter no sort", browse.ArtistsView{Level: arttui.ArtistLevel}, 1},
		{"artist level with filter (strip absorbs)", browse.ArtistsView{Level: arttui.ArtistLevel, Filter: filter.State{Active: true}}, 1},
		{"artist level with sort (strip absorbs)", browse.ArtistsView{Level: arttui.ArtistLevel, SortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"artist level with filter and sort (strip absorbs)", browse.ArtistsView{Level: arttui.ArtistLevel, Filter: filter.State{Active: true}, SortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"album level no sort", browse.ArtistsView{Level: arttui.AlbumLevel}, 2},
		{"album level with sort (strip absorbs)", browse.ArtistsView{Level: arttui.AlbumLevel, SortState: sorting.SortState{Field: sorting.SortName}}, 2},
		{"track level no sort", browse.ArtistsView{Level: arttui.TrackLevel}, 3},
		{"track level with sort (strip absorbs)", browse.ArtistsView{Level: arttui.TrackLevel, SortState: sorting.SortState{Field: sorting.SortTitle}}, 3},
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
		v    browse.AlbumsView
		want int
	}{
		{"album level no filter no sort", browse.AlbumsView{Level: albtui.AlbumLevel}, 1},
		{"album level with filter (strip absorbs)", browse.AlbumsView{Level: albtui.AlbumLevel, Filter: filter.State{Active: true}}, 1},
		{"album level with sort (strip absorbs)", browse.AlbumsView{Level: albtui.AlbumLevel, SortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"track level no sort", browse.AlbumsView{Level: albtui.TrackLevel}, 3},
		{"track level with sort (strip absorbs)", browse.AlbumsView{Level: albtui.TrackLevel, SortState: sorting.SortState{Field: sorting.SortTitle}}, 3},
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
