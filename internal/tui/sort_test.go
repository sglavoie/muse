package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestCycleSortField(t *testing.T) {
	fields := []SortField{SortDefault, SortArtist, SortTitle, SortAlbum}

	tests := []struct {
		name    string
		current SortField
		want    SortField
	}{
		{"default to artist", SortDefault, SortArtist},
		{"artist to title", SortArtist, SortTitle},
		{"title to album", SortTitle, SortAlbum},
		{"album wraps to default", SortAlbum, SortDefault},
		{"not in cycle returns first", SortDuration, SortDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CycleSortField(tt.current, fields)
			if got != tt.want {
				t.Errorf("CycleSortField(%d, fields) = %d, want %d", tt.current, got, tt.want)
			}
		})
	}
}

func TestCycleSortFieldSingleElement(t *testing.T) {
	fields := []SortField{SortDefault}
	got := CycleSortField(SortDefault, fields)
	if got != SortDefault {
		t.Errorf("single-element cycle: got %d, want %d", got, SortDefault)
	}
}

func TestCycleSortFieldEmpty(t *testing.T) {
	got := CycleSortField(SortDefault, nil)
	if got != SortDefault {
		t.Errorf("empty cycle: got %d, want %d", got, SortDefault)
	}
}

func TestSortTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Charlie", Title: "Zulu", Album: "Red", Duration: 300},
		{Artist: "Alpha", Title: "Bravo", Album: "Blue", Duration: 100},
		{Artist: "", Title: "Unknown", Album: "", Duration: 200},
		{Artist: "Bravo", Title: "Alpha", Album: "Green", Duration: 150},
	}

	t.Run("sort by artist asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4 tracks, got %d", len(sorted))
		}
		// Alpha, Bravo, Charlie, then unknown (empty)
		if sorted[0].Artist != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2].Artist)
		}
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortDesc)
		// Charlie, Bravo, Alpha, then unknown (empty)
		if sorted[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2].Artist)
		}
		// Unknown still last
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by title asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortTitle, SortAsc)
		if sorted[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Title)
		}
		if sorted[1].Title != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Title)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortAsc)
		if sorted[0].Duration != 100 {
			t.Errorf("first duration should be 100, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 300 {
			t.Errorf("last duration should be 300, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by duration desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortDesc)
		if sorted[0].Duration != 300 {
			t.Errorf("first duration should be 300, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 100 {
			t.Errorf("last duration should be 100, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by album asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortAlbum, SortAsc)
		if sorted[0].Album != "Blue" {
			t.Errorf("first should be Blue, got %q", sorted[0].Album)
		}
		// Empty album last
		if sorted[3].Album != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Album)
		}
	})

	t.Run("sort default returns nil", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDefault, SortAsc)
		if sorted != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("sort empty tracks returns nil", func(t *testing.T) {
		sorted := SortTracks(nil, SortArtist, SortAsc)
		if sorted != nil {
			t.Error("empty tracks should return nil")
		}
	})

	t.Run("original slice not modified", func(t *testing.T) {
		_ = SortTracks(tracks, SortArtist, SortAsc)
		if tracks[0].Artist != "Charlie" {
			t.Error("original slice was modified")
		}
	})
}

func TestSortFieldLabel(t *testing.T) {
	tests := []struct {
		field SortField
		want  string
	}{
		{SortDefault, "default"},
		{SortArtist, "Artist"},
		{SortTitle, "Title"},
		{SortAlbum, "Album"},
		{SortDuration, "Duration"},
		{SortName, "Name"},
		{SortTrackCount, "Track Count"},
		{SortSource, "Source"},
	}
	for _, tt := range tests {
		got := SortFieldLabel(tt.field)
		if got != tt.want {
			t.Errorf("SortFieldLabel(%d) = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestSortDirIndicator(t *testing.T) {
	if got := SortDirIndicator(SortAsc); got != "▲" {
		t.Errorf("SortAsc indicator = %q, want ▲", got)
	}
	if got := SortDirIndicator(SortDesc); got != "▼" {
		t.Errorf("SortDesc indicator = %q, want ▼", got)
	}
}

func TestSortQueueIndices(t *testing.T) {
	entries := []QueueEntry{
		{Title: "Zulu", Artist: "Charlie", Album: "Red", DurationMs: 3000, SourcePlaylist: "Rock"},
		{Title: "Alpha", Artist: "Bravo", Album: "Blue", DurationMs: 1000, SourcePlaylist: "Jazz"},
		{Title: "Mike", Artist: "Alpha", Album: "", DurationMs: 2000, SourcePlaylist: ""},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortTitle, SortAsc)
		if order == nil {
			t.Fatal("expected non-nil order")
		}
		// Alpha(1), Mike(2), Zulu(0)
		if entries[order[0]].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "Mike" {
			t.Errorf("second should be Mike, got %q", entries[order[1]].Title)
		}
		if entries[order[2]].Title != "Zulu" {
			t.Errorf("third should be Zulu, got %q", entries[order[2]].Title)
		}
	})

	t.Run("sort by title desc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortTitle, SortDesc)
		if entries[order[0]].Title != "Zulu" {
			t.Errorf("first should be Zulu, got %q", entries[order[0]].Title)
		}
		if entries[order[2]].Title != "Alpha" {
			t.Errorf("last should be Alpha, got %q", entries[order[2]].Title)
		}
	})

	t.Run("sort by artist asc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortArtist, SortAsc)
		if entries[order[0]].Artist != "Alpha" {
			t.Errorf("first should be Alpha, got %q", entries[order[0]].Artist)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDuration, SortAsc)
		if entries[order[0]].DurationMs != 1000 {
			t.Errorf("first should have 1000ms, got %d", entries[order[0]].DurationMs)
		}
		if entries[order[2]].DurationMs != 3000 {
			t.Errorf("last should have 3000ms, got %d", entries[order[2]].DurationMs)
		}
	})

	t.Run("sort by album asc unknown last", func(t *testing.T) {
		order := SortQueueIndices(entries, SortAlbum, SortAsc)
		// Blue(1), Red(0), then empty(2)
		if entries[order[0]].Album != "Blue" {
			t.Errorf("first should be Blue, got %q", entries[order[0]].Album)
		}
		if entries[order[1]].Album != "Red" {
			t.Errorf("second should be Red, got %q", entries[order[1]].Album)
		}
		if entries[order[2]].Album != "" {
			t.Errorf("third should be empty, got %q", entries[order[2]].Album)
		}
	})

	t.Run("sort by source asc unknown last", func(t *testing.T) {
		order := SortQueueIndices(entries, SortSource, SortAsc)
		// Jazz, Rock, then empty
		if entries[order[0]].SourcePlaylist != "Jazz" {
			t.Errorf("first should be Jazz, got %q", entries[order[0]].SourcePlaylist)
		}
		if entries[order[1]].SourcePlaylist != "Rock" {
			t.Errorf("second should be Rock, got %q", entries[order[1]].SourcePlaylist)
		}
		if entries[order[2]].SourcePlaylist != "" {
			t.Errorf("third should be empty, got %q", entries[order[2]].SourcePlaylist)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDefault, SortAsc)
		if order != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("empty entries returns nil", func(t *testing.T) {
		order := SortQueueIndices(nil, SortTitle, SortAsc)
		if order != nil {
			t.Error("empty entries should return nil")
		}
	})

	t.Run("original entries not modified", func(t *testing.T) {
		_ = SortQueueIndices(entries, SortTitle, SortAsc)
		if entries[0].Title != "Zulu" {
			t.Error("original entries were modified")
		}
	})
}

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
		{ID: 1, Title: "Zulu", Artist: "Charlie", DurationMs: 3000},
		{ID: 2, Title: "Alpha", Artist: "Bravo", DurationMs: 1000},
		{ID: 3, Title: "Mike", Artist: "Alpha", DurationMs: 2000},
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

	t.Run("default returns nil", func(t *testing.T) {
		order := SortPlaylistTrackIndices(tracks, SortDefault, SortAsc)
		if order != nil {
			t.Error("SortDefault should return nil")
		}
	})
}

func TestQueueEntryIndex(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
	}

	t.Run("no display order returns identity", func(t *testing.T) {
		if v.entryIndex(0) != 0 {
			t.Error("expected 0")
		}
		if v.entryIndex(2) != 2 {
			t.Error("expected 2")
		}
	})

	t.Run("with display order translates", func(t *testing.T) {
		v.displayOrder = []int{2, 0, 1} // C, A, B
		if v.entryIndex(0) != 2 {
			t.Errorf("display 0 should map to entry 2, got %d", v.entryIndex(0))
		}
		if v.entryIndex(1) != 0 {
			t.Errorf("display 1 should map to entry 0, got %d", v.entryIndex(1))
		}
		if v.entryIndex(2) != 1 {
			t.Errorf("display 2 should map to entry 1, got %d", v.entryIndex(2))
		}
	})
}

func TestQueueMoveDisabledWhenSorted(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
		},
		cursor:       0,
		displayOrder: []int{1, 0}, // sorted
	}

	v2, cmd := v.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Error("move down when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move down when sorted: expected TransientNotifyMsg")
	}
	_ = v2

	v.cursor = 1
	v3, cmd := v.HandleAction(actionMoveTrackUp)
	if cmd == nil {
		t.Error("move up when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move up when sorted: expected TransientNotifyMsg")
	}
	_ = v3
}

func TestSortStrings(t *testing.T) {
	items := []string{"Charlie", "Alpha", "", "Bravo"}

	t.Run("sort asc", func(t *testing.T) {
		sorted := SortStrings(items, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4, got %d", len(sorted))
		}
		// Alpha, Bravo, Charlie, then empty (unknown last)
		if sorted[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("sort desc", func(t *testing.T) {
		sorted := SortStrings(items, SortDesc)
		// Charlie, Bravo, Alpha, then empty
		if sorted[0] != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		if SortStrings(nil, SortAsc) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("original not modified", func(t *testing.T) {
		_ = SortStrings(items, SortAsc)
		if items[0] != "Charlie" {
			t.Error("original slice was modified")
		}
	})
}

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
		v2.level = 1 // AlbumLevel
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
		v2.level = 1 // TrackLevel
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
			level:     1, // AlbumLevel
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
			level: 2, // TrackLevel
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
			level: 2, // TrackLevel
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
			level: 1, // TrackLevel
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

// ---------------------------------------------------------------------------
// Session 5: Comprehensive sort integration tests
// ---------------------------------------------------------------------------

func TestSortCycleLibraryAllFields(t *testing.T) {
	// Cycle through all library sort fields and verify the order changes each time.
	tracks := []library.Track{
		{Hash: "1", Artist: "Charlie", Title: "Zulu", Album: "Red", Duration: 300},
		{Hash: "2", Artist: "Alpha", Title: "Bravo", Album: "Blue", Duration: 100},
		{Hash: "3", Artist: "Bravo", Title: "Alpha", Album: "Green", Duration: 150},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	v := LibraryView{
		index:     idx,
		sortState: SortState{Field: SortDefault, Dir: SortAsc},
	}

	// Cycle: Default → Artist
	v.sortState.Field = CycleSortField(v.sortState.Field, librarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != SortArtist {
		t.Fatalf("expected SortArtist, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Artist != "Alpha" {
		t.Errorf("first track by artist should be Alpha, got %q", v.sortedTracks[0].Artist)
	}

	// Cycle: Artist → Title
	v.sortState.Field = CycleSortField(v.sortState.Field, librarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != SortTitle {
		t.Fatalf("expected SortTitle, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Title != "Alpha" {
		t.Errorf("first track by title should be Alpha, got %q", v.sortedTracks[0].Title)
	}

	// Cycle: Title → Album
	v.sortState.Field = CycleSortField(v.sortState.Field, librarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != SortAlbum {
		t.Fatalf("expected SortAlbum, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Album != "Blue" {
		t.Errorf("first track by album should be Blue, got %q", v.sortedTracks[0].Album)
	}

	// Cycle: Album → Duration
	v.sortState.Field = CycleSortField(v.sortState.Field, librarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != SortDuration {
		t.Fatalf("expected SortDuration, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Duration != 100 {
		t.Errorf("first track by duration should be 100, got %f", v.sortedTracks[0].Duration)
	}

	// Cycle: Duration → Default (wraps)
	v.sortState.Field = CycleSortField(v.sortState.Field, librarySortFields)
	if v.sortState.Field != SortDefault {
		t.Fatalf("expected SortDefault after full cycle, got %d", v.sortState.Field)
	}
	v = v.recomputeSortedTracks()
	if v.sortedTracks != nil {
		t.Error("sortedTracks should be nil when SortDefault")
	}
}

func TestSortDirectionToggle(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "Alpha", Title: "A", Duration: 100},
		{Hash: "2", Artist: "Charlie", Title: "C", Duration: 300},
		{Hash: "3", Artist: "Bravo", Title: "B", Duration: 200},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	v := LibraryView{
		index:     idx,
		sortState: SortState{Field: SortArtist, Dir: SortAsc},
	}
	v = v.recomputeSortedTracks()

	// Ascending: Alpha, Bravo, Charlie
	if v.sortedTracks[0].Artist != "Alpha" {
		t.Errorf("asc first should be Alpha, got %q", v.sortedTracks[0].Artist)
	}
	if v.sortedTracks[2].Artist != "Charlie" {
		t.Errorf("asc last should be Charlie, got %q", v.sortedTracks[2].Artist)
	}

	// Switch to descending
	v.sortState.Dir = SortDesc
	v = v.recomputeSortedTracks()
	if v.sortedTracks[0].Artist != "Charlie" {
		t.Errorf("desc first should be Charlie, got %q", v.sortedTracks[0].Artist)
	}
	if v.sortedTracks[2].Artist != "Alpha" {
		t.Errorf("desc last should be Alpha, got %q", v.sortedTracks[2].Artist)
	}
}

func TestSortResetOnViewSwitch(t *testing.T) {
	// Simulates the reset that happens in root_sort.go when switching views.
	m := Model{
		activeView: viewLibrary,
		library: LibraryView{
			sortState: SortState{Field: SortArtist, Dir: SortDesc},
			sortedTracks: []library.Track{
				{Artist: "X"},
			},
		},
		queue: QueueView{
			sortState:    SortState{Field: SortTitle, Dir: SortAsc},
			displayOrder: []int{1, 0},
		},
	}

	m = m.resetLibrarySort()
	if m.library.sortState.Field != SortDefault {
		t.Error("library sort should be reset")
	}
	if m.library.sortedTracks != nil {
		t.Error("library sortedTracks should be nil")
	}

	m = m.resetQueueSort()
	if m.queue.sortState.Field != SortDefault {
		t.Error("queue sort should be reset")
	}
	if m.queue.displayOrder != nil {
		t.Error("queue displayOrder should be nil")
	}
}

func TestResetAllSorts(t *testing.T) {
	m := Model{
		library:   LibraryView{sortState: SortState{Field: SortArtist}},
		queue:     QueueView{sortState: SortState{Field: SortTitle}},
		playlists: PlaylistsView{sortState: SortState{Field: SortName}},
		artists:   ArtistsView{sortState: SortState{Field: SortName}},
		albums:    AlbumsView{sortState: SortState{Field: SortTitle}},
	}

	m = m.resetAllSorts()

	if m.library.sortState.Field != SortDefault {
		t.Error("library sort not reset")
	}
	if m.queue.sortState.Field != SortDefault {
		t.Error("queue sort not reset")
	}
	if m.playlists.sortState.Field != SortDefault {
		t.Error("playlists sort not reset")
	}
	if m.artists.sortState.Field != SortDefault {
		t.Error("artists sort not reset")
	}
	if m.albums.sortState.Field != SortDefault {
		t.Error("albums sort not reset")
	}
}

func TestQueueSortDequeueCorrectEntry(t *testing.T) {
	// Sort queue by title, verify dequeue targets the correct underlying entry.
	v := QueueView{
		entries: []QueueEntry{
			{ID: 10, Title: "Zulu", Artist: "A"},
			{ID: 20, Title: "Alpha", Artist: "B"},
			{ID: 30, Title: "Mike", Artist: "C"},
		},
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v = v.recomputeDisplayOrder()

	// Display order should be: Alpha(1), Mike(2), Zulu(0)
	if v.displayOrder == nil {
		t.Fatal("displayOrder should not be nil")
	}

	// Cursor at display row 0 → should map to entry index 1 (Alpha)
	v.cursor = 0
	_, cmd := v.HandleAction(actionDequeue)
	if cmd == nil {
		t.Fatal("expected a cmd for dequeue")
	}
	msg := cmd()
	deqMsg, ok := msg.(DequeueTrackMsg)
	if !ok {
		t.Fatalf("expected DequeueTrackMsg, got %T", msg)
	}
	if v.entries[deqMsg.Index].Title != "Alpha" {
		t.Errorf("expected to dequeue Alpha (entry 1), got %q at index %d",
			v.entries[deqMsg.Index].Title, deqMsg.Index)
	}

	// Cursor at display row 2 → should map to entry index 0 (Zulu)
	v.cursor = 2
	_, cmd = v.HandleAction(actionDequeue)
	msg = cmd()
	deqMsg = msg.(DequeueTrackMsg)
	if v.entries[deqMsg.Index].Title != "Zulu" {
		t.Errorf("expected to dequeue Zulu (entry 0), got %q at index %d",
			v.entries[deqMsg.Index].Title, deqMsg.Index)
	}
}

func TestQueueSortDisablesMoveUpDown(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
		displayOrder: []int{2, 1, 0}, // sorted
		cursor:       0,
	}

	_, cmd := v.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Error("move down when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move down when sorted: expected TransientNotifyMsg")
	}

	v.cursor = 2
	_, cmd = v.HandleAction(actionMoveTrackUp)
	if cmd == nil {
		t.Error("move up when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move up when sorted: expected TransientNotifyMsg")
	}
}

func TestQueueSortSelectCorrectEntry(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{ID: 1, Hash: "h1", Title: "Zulu"},
			{ID: 2, Hash: "h2", Title: "Alpha"},
		},
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v = v.recomputeDisplayOrder()

	// Display: Alpha(1), Zulu(0)
	v.cursor = 0 // Alpha
	_, cmd := v.HandleAction(actionSelect)
	if cmd == nil {
		t.Fatal("expected cmd for select")
	}
	msg := cmd()
	playMsg, ok := msg.(PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	// Should play entry 1 (Alpha), not 0 (Zulu)
	if playMsg.Index != 1 {
		t.Errorf("expected play index 1 (Alpha), got %d", playMsg.Index)
	}
}

func TestQueueBulkSelectionToggleWithSort(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{ID: 1, Title: "Zulu"},
			{ID: 2, Title: "Alpha"},
		},
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v = v.recomputeDisplayOrder()

	// Display: Alpha(1), Zulu(0)
	v.cursor = 0 // Alpha
	_, cmd := v.HandleAction(actionBulkSelectionToggle)
	if cmd == nil {
		t.Fatal("expected cmd for bulk selection toggle")
	}
	msg := cmd()
	bulkMsg, ok := msg.(BulkSelectionToggleMsg)
	if !ok {
		t.Fatalf("expected BulkSelectionToggleMsg, got %T", msg)
	}
	// Should toggle entry 1 (Alpha), not 0 (Zulu)
	if bulkMsg.Index != 1 {
		t.Errorf("expected toggle index 1 (Alpha), got %d", bulkMsg.Index)
	}
}

func TestSortResetOnArtistsDrillDown(t *testing.T) {
	v := ArtistsView{
		artists:   []string{"Charlie", "Alpha", "Bravo"},
		sortState: SortState{Field: SortName, Dir: SortAsc},
	}

	// Sort artists
	v = v.recomputeArtistsSort()
	if v.artists[0] != "Alpha" {
		t.Fatalf("expected sorted first to be Alpha, got %q", v.artists[0])
	}

	// Drill down to albums → sort resets
	v2 := v.openAlbums([]string{"Album1", "Album2"})
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openAlbums")
	}

	// Set sort at album level
	v2.sortState = SortState{Field: SortName, Dir: SortDesc}

	// Drill down to tracks → sort resets
	v3 := v2.openTracks([]library.Track{{Title: "T1"}})
	if v3.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}

	// Set sort at track level
	v3.sortState = SortState{Field: SortTitle, Dir: SortAsc}

	// Go back → sort resets
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

	// Drill down to tracks → sort resets
	v2 := v.openTracks([]library.Track{{Title: "T1"}})
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}

	// Set sort and go back → sort resets
	v2.sortState = SortState{Field: SortTitle, Dir: SortDesc}
	v3 := v2.back()
	if v3.sortState.Field != SortDefault {
		t.Error("sort should reset on back")
	}
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

	// Drill down to tracks → sort resets
	tracks := []db.PlaylistTrackRow{{ID: 1, Title: "T1"}}
	v2 := v.openTracks(tracks)
	if v2.sortState.Field != SortDefault {
		t.Error("sort should reset on openTracks")
	}
	if v2.sortedPlaylists != nil {
		t.Error("sortedPlaylists should be nil after openTracks")
	}

	// Set track sort, go back → sort resets
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

func TestSortNotification(t *testing.T) {
	tests := []struct {
		state SortState
		want  string
	}{
		{SortState{Field: SortDefault}, "Sort: default"},
		{SortState{Field: SortArtist, Dir: SortAsc}, "Sort: Artist ▲"},
		{SortState{Field: SortTitle, Dir: SortDesc}, "Sort: Title ▼"},
		{SortState{Field: SortDuration, Dir: SortAsc}, "Sort: Duration ▲"},
		{SortState{Field: SortName, Dir: SortDesc}, "Sort: Name ▼"},
		{SortState{Field: SortTrackCount, Dir: SortAsc}, "Sort: Track Count ▲"},
		{SortState{Field: SortSource, Dir: SortDesc}, "Sort: Source ▼"},
	}
	for _, tt := range tests {
		got := sortNotification(tt.state)
		if got != tt.want {
			t.Errorf("sortNotification(%+v) = %q, want %q", tt.state, got, tt.want)
		}
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

func TestAdjustScroll(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		maxRows      int
		want         int
	}{
		{"cursor above viewport", 2, 5, 10, 2},
		{"cursor below viewport", 15, 0, 10, 6},
		{"cursor within viewport", 5, 0, 10, 0},
		{"cursor at viewport bottom edge", 9, 0, 10, 0},
		{"zero maxRows", 5, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustScroll(tt.cursor, tt.scrollOffset, tt.maxRows)
			if got != tt.want {
				t.Errorf("adjustScroll(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.maxRows, got, tt.want)
			}
		})
	}
}

func TestFindStringPosition(t *testing.T) {
	items := []string{"Alpha", "Bravo", "Charlie"}
	if findStringPosition(items, "Bravo") != 1 {
		t.Error("expected 1 for Bravo")
	}
	if findStringPosition(items, "Missing") != 0 {
		t.Error("expected 0 for missing item")
	}
	if findStringPosition(nil, "A") != 0 {
		t.Error("expected 0 for nil items")
	}
}

func TestFindTrackHashPosition(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a"},
		{Hash: "b"},
		{Hash: "c"},
	}
	if findTrackHashPosition(tracks, "b") != 1 {
		t.Error("expected 1 for hash b")
	}
	if findTrackHashPosition(tracks, "missing") != 0 {
		t.Error("expected 0 for missing hash")
	}
	if findTrackHashPosition(tracks, "") != 0 {
		t.Error("expected 0 for empty hash")
	}
}

func TestFindPlaylistPosition(t *testing.T) {
	pls := []db.PlaylistRow{
		{ID: 10, Name: "A"},
		{ID: 20, Name: "B"},
		{ID: 30, Name: "C"},
	}
	if findPlaylistPosition(pls, 20) != 1 {
		t.Error("expected 1 for ID 20")
	}
	if findPlaylistPosition(pls, 99) != 0 {
		t.Error("expected 0 for missing ID")
	}
}

func TestQueueFindEntryDisplayPosition(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{{Title: "A"}, {Title: "B"}, {Title: "C"}},
	}

	// No display order → identity
	if v.findEntryDisplayPosition(2) != 2 {
		t.Error("expected identity mapping without display order")
	}

	// With display order
	v.displayOrder = []int{2, 0, 1}
	if got := v.findEntryDisplayPosition(0); got != 1 {
		t.Errorf("entry 0 should be at display 1, got %d", got)
	}
	if got := v.findEntryDisplayPosition(2); got != 0 {
		t.Errorf("entry 2 should be at display 0, got %d", got)
	}
	// Missing entry
	if got := v.findEntryDisplayPosition(99); got != 0 {
		t.Errorf("missing entry should return 0, got %d", got)
	}
}

func TestPlaylistsFindTrackDisplayPosition(t *testing.T) {
	v := PlaylistsView{
		tracks: []db.PlaylistTrackRow{{ID: 1}, {ID: 2}, {ID: 3}},
	}

	// No sort order → identity
	if v.findTrackDisplayPosition(1) != 1 {
		t.Error("expected identity mapping without sort order")
	}

	// With sort order
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

func TestCompareStringsUnknownLast(t *testing.T) {
	// Unknown values always sort last regardless of direction.
	tests := []struct {
		a, b string
		dir  SortDir
		want bool
	}{
		{"Alpha", "Bravo", SortAsc, true},
		{"Alpha", "Bravo", SortDesc, false},
		{"", "Alpha", SortAsc, false},                // empty is unknown → sorts after
		{"", "Alpha", SortDesc, false},               // still after
		{"Alpha", "", SortAsc, true},                 // known before unknown
		{"Alpha", "", SortDesc, true},                // still before
		{"", "", SortAsc, false},                     // both unknown → stable
		{"Unknown Artist", "Alpha", SortAsc, false},  // sentinel unknown
		{"Unknown Artist", "Alpha", SortDesc, false}, // still after
	}
	for _, tt := range tests {
		got := compareStringsUnknownLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("compareStringsUnknownLast(%q, %q, %d) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestSortCycleQueueAllFields(t *testing.T) {
	state := SortState{Field: SortDefault}

	// Default → Title → Artist → Album → Duration → Source → Default
	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortSource, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, queueSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistListFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortName, SortTrackCount, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistListSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistTrackFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortTitle, SortArtist, SortDuration, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistTrackSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortTracksStableOrder(t *testing.T) {
	// Tracks with identical sort keys should preserve relative order.
	tracks := []library.Track{
		{Hash: "1", Artist: "Same", Title: "B"},
		{Hash: "2", Artist: "Same", Title: "A"},
		{Hash: "3", Artist: "Same", Title: "C"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	// All have same artist, so order should be preserved (stable sort).
	if sorted[0].Hash != "1" || sorted[1].Hash != "2" || sorted[2].Hash != "3" {
		t.Error("stable sort should preserve order for equal keys")
	}
}

func TestSortTracksCaseInsensitive(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "bravo"},
		{Hash: "2", Artist: "Alpha"},
		{Hash: "3", Artist: "CHARLIE"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	if sorted[0].Artist != "Alpha" || sorted[1].Artist != "bravo" || sorted[2].Artist != "CHARLIE" {
		t.Errorf("case-insensitive sort failed: got %q, %q, %q",
			sorted[0].Artist, sorted[1].Artist, sorted[2].Artist)
	}
}

func TestQueueEntryIndexBoundsCheck(t *testing.T) {
	v := QueueView{
		entries:      []QueueEntry{{Title: "A"}, {Title: "B"}},
		displayOrder: []int{1, 0},
	}

	// Out of bounds should return the index as-is (safety fallback).
	if got := v.entryIndex(-1); got != -1 {
		t.Errorf("negative index: got %d, want -1", got)
	}
	if got := v.entryIndex(5); got != 5 {
		t.Errorf("over-bounds index: got %d, want 5", got)
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

func TestSortKeybindingsRegistered(t *testing.T) {
	reg := newRegistry()

	tests := []struct {
		key  string
		want action
	}{
		{"ctrl+s", actionSortCycle},
		{"ctrl+a", actionSortAsc},
		{"ctrl+d", actionSortDescOrPageDown},
	}
	for _, tt := range tests {
		got := reg.resolve(contextGlobal, tt.key)
		if got != tt.want {
			t.Errorf("resolve(contextGlobal, %q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestCtrlDResolvesCorrectlyPerContext(t *testing.T) {
	reg := newRegistry()

	// ctrl+d should resolve to actionSortDescOrPageDown in all view contexts
	// because it's registered globally and not overridden per-view.
	contexts := []struct {
		name string
		ctx  bindingContext
	}{
		{"library", contextLibrary},
		{"queue", contextQueue},
		{"playlists", contextPlaylists},
		{"artists", contextArtists},
		{"albums", contextAlbums},
		{"global", contextGlobal},
	}

	for _, tc := range contexts {
		got := reg.resolve(tc.ctx, "ctrl+d")
		if got != actionSortDescOrPageDown {
			t.Errorf("resolve(%s, ctrl+d) = %v, want actionSortDescOrPageDown", tc.name, got)
		}
	}
}

func TestArtistsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    ArtistsView
		want int
	}{
		{
			"artist level no filter no sort",
			ArtistsView{level: arttui.ArtistLevel},
			0,
		},
		{
			"artist level with filter",
			ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}},
			1,
		},
		{
			"artist level with sort",
			ArtistsView{level: arttui.ArtistLevel, sortState: SortState{Field: SortName}},
			1,
		},
		{
			"artist level with filter and sort",
			ArtistsView{level: arttui.ArtistLevel, filter: filter.State{Active: true}, sortState: SortState{Field: SortName}},
			2,
		},
		{
			"album level no sort",
			ArtistsView{level: arttui.AlbumLevel},
			3,
		},
		{
			"album level with sort",
			ArtistsView{level: arttui.AlbumLevel, sortState: SortState{Field: SortName}},
			4,
		},
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
		{
			"album level no filter no sort",
			AlbumsView{level: albtui.AlbumLevel},
			0,
		},
		{
			"album level with filter",
			AlbumsView{level: albtui.AlbumLevel, filter: filter.State{Active: true}},
			1,
		},
		{
			"album level with sort",
			AlbumsView{level: albtui.AlbumLevel, sortState: SortState{Field: SortName}},
			1,
		},
		{
			"track level no sort",
			AlbumsView{level: albtui.TrackLevel},
			3,
		},
		{
			"track level with sort",
			AlbumsView{level: albtui.TrackLevel, sortState: SortState{Field: SortTitle}},
			4,
		},
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
		{
			"category level",
			FavoritesView{level: favtui.CategoryLevel},
			0,
		},
		{
			"list level artists no filter no sort",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists},
			0,
		},
		{
			"list level artists with filter",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}},
			1,
		},
		{
			"list level artists with committed filter text",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Text: "foo"}},
			1,
		},
		{
			"list level artists with sort",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, sortState: SortState{Field: SortName}},
			1,
		},
		{
			"list level artists with filter and sort",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryArtists, filter: filter.State{Active: true}, sortState: SortState{Field: SortName}},
			2,
		},
		{
			"list level tracks no filter no sort",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks},
			1,
		},
		{
			"list level tracks with filter",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, filter: filter.State{Active: true}},
			2,
		},
		{
			"list level tracks with sort",
			FavoritesView{level: favtui.ListLevel, category: favtui.CategoryTracks, sortState: SortState{Field: SortTitle}},
			2,
		},
		{
			"track level no sort",
			FavoritesView{level: favtui.TrackLevel},
			3,
		},
		{
			"track level with sort",
			FavoritesView{level: favtui.TrackLevel, sortState: SortState{Field: SortTitle}},
			4,
		},
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
