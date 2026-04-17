package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestArtistsViewSortReset(t *testing.T) {
	v := ArtistsView{
		artists:   []string{"Charlie", "Alpha", "Bravo"},
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}

	t.Run("openAlbums resets sort", func(t *testing.T) {
		v2 := v.openAlbums([]string{"Album1"})
		if v2.sortState.Field != SortDefault {
			t.Error("sort should be reset on openAlbums")
		}
	})

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.openTracks([]library.Track{{Title: "T1"}})
		if v2.sortState.Field != SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.level = 1
		v2.albums = []string{"Album1"}
		v2.sortState = SortState{Field: SortName, Dir: SortDesc}
		v3 := v2.back()
		if v3.sortState.Field != SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestAlbumsViewSortReset(t *testing.T) {
	v := AlbumsView{
		albums:    []string{"Zulu", "Alpha"},
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}

	t.Run("openTracks resets sort", func(t *testing.T) {
		v2 := v.openTracks([]library.Track{{Title: "T1"}})
		if v2.sortState.Field != SortDefault {
			t.Error("sort should be reset on openTracks")
		}
	})

	t.Run("back resets sort", func(t *testing.T) {
		v2 := v
		v2.level = 1
		v2.tracks = []library.Track{{Title: "T1"}}
		v2.sortState = SortState{Field: SortTitle, Dir: SortDesc}
		v3 := v2.back()
		if v3.sortState.Field != SortDefault {
			t.Error("sort should be reset on back")
		}
	})
}

func TestArtistsRecomputeSort(t *testing.T) {
	t.Run("sort artists by name asc", func(t *testing.T) {
		v := ArtistsView{
			artists:   []string{"Charlie", "Alpha", "Bravo"},
			sortState: SortState{Field: SortName, Dir: SortAsc},
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
			sortState: SortState{Field: SortName, Dir: SortDesc},
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
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
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
			sortState: SortState{Field: SortDuration, Dir: SortDesc},
		}
		v = v.recomputeArtistsSort()
		if v.tracks[0].Duration != 300 {
			t.Errorf("first should have duration 300, got %f", v.tracks[0].Duration)
		}
	})

	t.Run("default is no-op", func(t *testing.T) {
		v := ArtistsView{
			artists:   []string{"Charlie", "Alpha"},
			sortState: SortState{Field: SortDefault},
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
			sortState: SortState{Field: SortName, Dir: SortAsc},
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
			sortState: SortState{Field: SortTitle, Dir: SortDesc},
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
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}

	v = v.recomputeArtistsSort()
	if v.artists[0] != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.artists[0])
	}

	v2 := v.openAlbums([]string{"Album1", "Album2"})
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openAlbums")
	}

	v2.sortState = SortState{Field: SortName, Dir: SortDesc}
	v3 := v2.openTracks([]library.Track{{Title: "T1"}})
	if v3.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v3.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	v4 := v3.back()
	if v4.sortState.Field != SortDefault {
		t.Error("sort should reset on back")
	}
}

func TestSortResetOnAlbumsDrillDown(t *testing.T) {
	v := AlbumsView{
		albums:    []string{"Zulu", "Alpha"},
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}
	v = v.recomputeAlbumsSort()

	v2 := v.openTracks([]library.Track{{Title: "T1"}})
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}

	v2.sortState = SortState{Field: SortTitle, Dir: SortDesc}
	v3 := v2.back()
	if v3.sortState.Field != SortDefault {
		t.Error("sort should reset on back")
	}
}

func TestArtistsSortFieldsForLevel(t *testing.T) {
	tests := []struct {
		level arttui.Level
		want  []SortField
	}{
		{arttui.ArtistLevel, artistArtistSortFields},
		{arttui.AlbumLevel, artistAlbumSortFields},
		{arttui.TrackLevel, artistTrackSortFields},
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
		want  []SortField
	}{
		{albtui.AlbumLevel, albumAlbumSortFields},
		{albtui.TrackLevel, albumTrackSortFields},
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
		{"artist level no filter no sort", ArtistsView{level: arttui.ArtistLevel}, 0},
		{"artist level with filter", ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}}, 1},
		{"artist level with sort", ArtistsView{level: arttui.ArtistLevel, sortState: SortState{Field: SortName}}, 1},
		{"artist level with filter and sort", ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}, sortState: SortState{Field: SortName}}, 2},
		{"album level no sort", ArtistsView{level: arttui.AlbumLevel}, 3},
		{"album level with sort", ArtistsView{level: arttui.AlbumLevel, sortState: SortState{Field: SortName}}, 4},
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
		{"album level no filter no sort", AlbumsView{level: albtui.AlbumLevel}, 0},
		{"album level with filter", AlbumsView{level: albtui.AlbumLevel, filter: filter.State{Active: true}}, 1},
		{"album level with sort", AlbumsView{level: albtui.AlbumLevel, sortState: SortState{Field: SortName}}, 1},
		{"track level no sort", AlbumsView{level: albtui.TrackLevel}, 3},
		{"track level with sort", AlbumsView{level: albtui.TrackLevel, sortState: SortState{Field: SortTitle}}, 4},
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

func TestFavoritesVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    FavoritesView
		want int
	}{
		{"category level", FavoritesView{level: favtui.CategoryLevel}, 0},
		{"list level artists no filter no sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists}, 0},
		{"list level artists with filter", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}}, 1},
		{"list level artists with committed filter text", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Text: "foo"}}, 1},
		{"list level artists with sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, sortState: SortState{Field: SortName}}, 1},
		{"list level artists with filter and sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}, sortState: SortState{Field: SortName}}, 2},
		{"list level tracks no filter no sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks}, 1},
		{"list level tracks with filter", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, filter: filter.State{Active: true}}, 2},
		{"list level tracks with sort", FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, sortState: SortState{Field: SortTitle}}, 2},
		{"track level no sort", FavoritesView{level: favtui.TrackLevel}, 3},
		{"track level with sort", FavoritesView{level: favtui.TrackLevel, sortState: SortState{Field: SortTitle}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := favoritesVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("favoritesVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTagsSortFieldsForLevel(t *testing.T) {
	t.Run("TagLevel returns nil", func(t *testing.T) {
		got := tagsSortFieldsForLevel(tagtui.TagLevel)
		if got != nil {
			t.Errorf("expected nil for TagLevel, got %v", got)
		}
	})
	t.Run("TrackLevel returns tagsTrackSortFields", func(t *testing.T) {
		got := tagsSortFieldsForLevel(tagtui.TrackLevel)
		if len(got) != len(tagsTrackSortFields) {
			t.Fatalf("len=%d, want %d", len(got), len(tagsTrackSortFields))
		}
		for i, f := range got {
			if f != tagsTrackSortFields[i] {
				t.Errorf("idx %d: got %d, want %d", i, f, tagsTrackSortFields[i])
			}
		}
	})
}

func TestTagsRecomputeSort(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "Charlie", Album: "Red", DurationMs: 300000},
		{Hash: "h2", Title: "Alpha", Artist: "Alice", Album: "Blue", DurationMs: 100000},
		{Hash: "h3", Title: "Bravo", Artist: "Bob", Album: "Green", DurationMs: 200000},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder == nil {
			t.Fatal("expected non-nil sortedTrackOrder")
		}
		display := v.displayTracks()
		if display[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", display[0].Title)
		}
		if display[2].Title != "Zulu" {
			t.Errorf("last should be Zulu, got %q", display[2].Title)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortArtist, Dir: SortDesc},
		}
		v = v.recomputeTagsSort()
		display := v.displayTracks()
		if display[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", display[0].Artist)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortDuration, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		display := v.displayTracks()
		if display[0].DurationMs != 100000 {
			t.Errorf("first should have 100000ms, got %d", display[0].DurationMs)
		}
	})

	t.Run("default clears sortedTrackOrder", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortDefault},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil for SortDefault")
		}
	})

	t.Run("TagLevel clears sortedTrackOrder", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TagLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil at TagLevel")
		}
	})
}

func TestTagsSortResetOnBack(t *testing.T) {
	v := TagsView{
		level:     tagtui.TrackLevel,
		tracks:    []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}},
		sortState: SortState{Field: SortTitle, Dir: SortDesc},
	}
	v = v.recomputeTagsSort()
	if v.sortedTrackOrder == nil {
		t.Fatal("precondition: sortedTrackOrder should be non-nil")
	}

	v2 := v.back()
	if v2.sortState.Field != SortDefault {
		t.Error("sort should be reset on back")
	}
	if v2.sortedTrackOrder != nil {
		t.Error("sortedTrackOrder should be nil after back")
	}
}

func TestTagsSortResetOnOpenTracks(t *testing.T) {
	v := TagsView{
		level:     tagtui.TagLevel,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	tag := db.TagRow{ID: 1, Name: "MyTag"}
	tracks := []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}}
	v2 := v.openTracks(tag, tracks)
	if v2.sortState.Field != SortDefault {
		t.Error("sort should be reset on openTracks")
	}
}

func TestTagsFocusedTrackWithSort(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "C"},
		{Hash: "h2", Title: "Alpha", Artist: "A"},
	}
	v := TagsView{
		level:     tagtui.TrackLevel,
		tracks:    tracks,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
		cursor:    0,
	}
	v = v.recomputeTagsSort()

	track, ok := v.focusedTrack()
	if !ok {
		t.Fatal("expected focused track")
	}
	if track.Title != "Alpha" {
		t.Errorf("cursor 0 with title-asc sort should be Alpha, got %q", track.Title)
	}
}

func TestTagsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    TagsView
		want int
	}{
		{"tag level no filter no sort", TagsView{level: tagtui.TagLevel}, 0},
		{"tag level with filter", TagsView{level: tagtui.TagLevel, filter: filter.State{Active: true}}, 1},
		{"tag level with sort", TagsView{level: tagtui.TagLevel, sortState: SortState{Field: SortName}}, 1},
		{"track level no sort", TagsView{level: tagtui.TrackLevel}, 3},
		{"track level with sort", TagsView{level: tagtui.TrackLevel, sortState: SortState{Field: SortTitle}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("tagsVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}
