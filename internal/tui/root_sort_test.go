package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestSortCycleLibraryAllFields(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "Charlie", Title: "Zulu", Album: "Red", Duration: 300},
		{Hash: "2", Artist: "Alpha", Title: "Bravo", Album: "Blue", Duration: 100},
		{Hash: "3", Artist: "Bravo", Title: "Alpha", Album: "Green", Duration: 150},
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	v := libtui.View{
		Index:     idx,
		SortState: sorting.SortState{Field: sorting.SortDefault, Dir: sorting.SortAsc},
	}

	v.SortState.Field = sorting.CycleSortField(v.SortState.Field, sorting.LibrarySortFields)
	v = v.RecomputeSortedTracks()
	if v.SortState.Field != sorting.SortArtist {
		t.Fatalf("expected SortArtist, got %d", v.SortState.Field)
	}
	if v.SortedTracks[0].Artist != "Alpha" {
		t.Errorf("first track by artist should be Alpha, got %q", v.SortedTracks[0].Artist)
	}

	v.SortState.Field = sorting.CycleSortField(v.SortState.Field, sorting.LibrarySortFields)
	v = v.RecomputeSortedTracks()
	if v.SortState.Field != sorting.SortTitle {
		t.Fatalf("expected SortTitle, got %d", v.SortState.Field)
	}
	if v.SortedTracks[0].Title != "Alpha" {
		t.Errorf("first track by title should be Alpha, got %q", v.SortedTracks[0].Title)
	}

	v.SortState.Field = sorting.CycleSortField(v.SortState.Field, sorting.LibrarySortFields)
	v = v.RecomputeSortedTracks()
	if v.SortState.Field != sorting.SortAlbum {
		t.Fatalf("expected SortAlbum, got %d", v.SortState.Field)
	}
	if v.SortedTracks[0].Album != "Blue" {
		t.Errorf("first track by album should be Blue, got %q", v.SortedTracks[0].Album)
	}

	v.SortState.Field = sorting.CycleSortField(v.SortState.Field, sorting.LibrarySortFields)
	v = v.RecomputeSortedTracks()
	if v.SortState.Field != sorting.SortDuration {
		t.Fatalf("expected SortDuration, got %d", v.SortState.Field)
	}
	if v.SortedTracks[0].Duration != 100 {
		t.Errorf("first track by duration should be 100, got %f", v.SortedTracks[0].Duration)
	}

	v.SortState.Field = sorting.CycleSortField(v.SortState.Field, sorting.LibrarySortFields)
	if v.SortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after full cycle, got %d", v.SortState.Field)
	}
	v = v.RecomputeSortedTracks()
	if v.SortedTracks != nil {
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

	v := libtui.View{
		Index:     idx,
		SortState: sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc},
	}
	v = v.RecomputeSortedTracks()

	if v.SortedTracks[0].Artist != "Alpha" {
		t.Errorf("asc first should be Alpha, got %q", v.SortedTracks[0].Artist)
	}
	if v.SortedTracks[2].Artist != "Charlie" {
		t.Errorf("asc last should be Charlie, got %q", v.SortedTracks[2].Artist)
	}

	v.SortState.Dir = sorting.SortDesc
	v = v.RecomputeSortedTracks()
	if v.SortedTracks[0].Artist != "Charlie" {
		t.Errorf("desc first should be Charlie, got %q", v.SortedTracks[0].Artist)
	}
	if v.SortedTracks[2].Artist != "Alpha" {
		t.Errorf("desc last should be Alpha, got %q", v.SortedTracks[2].Artist)
	}
}

func TestSortResetOnViewSwitch(t *testing.T) {
	m := Model{
		ActiveView: viewLibrary,
		library: libtui.View{
			SortState: sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortDesc},
			SortedTracks: []library.Track{
				{Artist: "X"},
			},
		},
		queue: queue.View{
			SortState:    sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
			DisplayOrder: []int{1, 0},
		},
	}

	m = m.resetLibrarySort()
	if m.library.SortState.Field != sorting.SortDefault {
		t.Error("library sort should be reset")
	}
	if m.library.SortedTracks != nil {
		t.Error("library sortedTracks should be nil")
	}

	m = m.resetQueueSort()
	if m.queue.SortState.Field != sorting.SortDefault {
		t.Error("queue sort should be reset")
	}
	if m.queue.DisplayOrder != nil {
		t.Error("queue displayOrder should be nil")
	}
}

func TestResetAllSorts(t *testing.T) {
	m := Model{
		library:   libtui.View{SortState: sorting.SortState{Field: sorting.SortArtist}},
		queue:     queue.View{SortState: sorting.SortState{Field: sorting.SortTitle}},
		playlists: pltui.PlaylistsView{SortState: sorting.SortState{Field: sorting.SortName}},
		browse: browse.BrowseView{
			Artists: browse.ArtistsView{SortState: sorting.SortState{Field: sorting.SortName}},
			Albums:  browse.AlbumsView{SortState: sorting.SortState{Field: sorting.SortTitle}},
		},
	}

	m = m.resetAllSorts()

	if m.library.SortState.Field != sorting.SortDefault {
		t.Error("library sort not reset")
	}
	if m.queue.SortState.Field != sorting.SortDefault {
		t.Error("queue sort not reset")
	}
	if m.playlists.SortState.Field != sorting.SortDefault {
		t.Error("playlists sort not reset")
	}
	if m.browse.Artists.SortState.Field != sorting.SortDefault {
		t.Error("artists sort not reset")
	}
	if m.browse.Albums.SortState.Field != sorting.SortDefault {
		t.Error("albums sort not reset")
	}
}

func TestSortKeybindingsRegistered(t *testing.T) {
	reg := keys.NewRegistry()

	tests := []struct {
		key  string
		want keys.Action
	}{
		{"ctrl+s", keys.ActionSortCycle},
		{"shift+up", keys.ActionSortAsc},
		{"shift+down", keys.ActionSortDesc},
	}
	for _, tt := range tests {
		got := reg.Resolve(keys.ContextGlobal, tt.key)
		if got != tt.want {
			t.Errorf("resolve(contextGlobal, %q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestCtrlDResolvesCorrectlyPerContext(t *testing.T) {
	reg := keys.NewRegistry()

	contexts := []struct {
		name string
		ctx  keys.Context
	}{
		{"library", keys.ContextLibrary},
		{"library", keys.ContextLibrary},
		{"queue", keys.ContextQueue},
		{"playlists", keys.ContextPlaylists},
		{"artists", keys.ContextArtists},
		{"albums", keys.ContextAlbums},
		{"favorites", keys.ContextFavorites},
		{"genres", keys.ContextGenres},
	}

	for _, tc := range contexts {
		got := reg.Resolve(tc.ctx, "ctrl+d")
		if got != keys.ActionHalfPageDown {
			t.Errorf("resolve(%s, ctrl+d) = %v, want actionHalfPageDown", tc.name, got)
		}
	}
}

func TestSortCycleToDefaultRederivesArtists(t *testing.T) {
	m := Model{}
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.library.Index.Insert(library.Track{Artist: "B", Title: "T1", Path: "/b", Hash: "hb"})
	m.library.Index.Insert(library.Track{Artist: "A", Title: "T2", Path: "/a", Hash: "ha"})
	m.library.Index.Insert(library.Track{Artist: "C", Title: "T3", Path: "/c", Hash: "hc"})

	m.browse.Artists.Artists = arttui.DeriveArtists(m.library.Index.Tracks())
	original := make([]string, len(m.browse.Artists.Artists))
	copy(original, m.browse.Artists.Artists)

	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()

	if len(m.browse.Artists.Artists) > 1 && m.browse.Artists.Artists[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.Artists.SortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.Artists.SortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.Artists.Artists) {
			t.Fatalf("artists list too short: got %d, want %d", len(m.browse.Artists.Artists), len(original))
		}
		if m.browse.Artists.Artists[i] != want {
			t.Errorf("artists[%d] = %q, want %q", i, m.browse.Artists.Artists[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesAlbums(t *testing.T) {
	m := Model{}
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.library.Index.Insert(library.Track{Artist: "A", Album: "Zebra", Title: "T1", Path: "/1", Hash: "h1"})
	m.library.Index.Insert(library.Track{Artist: "A", Album: "Alpha", Title: "T2", Path: "/2", Hash: "h2"})
	m.library.Index.Insert(library.Track{Artist: "A", Album: "Mid", Title: "T3", Path: "/3", Hash: "h3"})

	m.browse.Albums.Albums = albtui.DeriveAllAlbums(m.library.Index.Tracks())
	original := make([]string, len(m.browse.Albums.Albums))
	copy(original, m.browse.Albums.Albums)

	m.browse.Albums.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()

	if len(m.browse.Albums.Albums) > 1 && m.browse.Albums.Albums[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.Albums.SortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.Albums.SortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.Albums.Albums) {
			t.Fatalf("albums list too short: got %d, want %d", len(m.browse.Albums.Albums), len(original))
		}
		if m.browse.Albums.Albums[i] != want {
			t.Errorf("albums[%d] = %q, want %q", i, m.browse.Albums.Albums[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesGenres(t *testing.T) {
	m := Model{}
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.library.Index.Insert(library.Track{Artist: "A", Genre: "Rock", Title: "T1", Path: "/1", Hash: "h1"})
	m.library.Index.Insert(library.Track{Artist: "A", Genre: "Jazz", Title: "T2", Path: "/2", Hash: "h2"})
	m.library.Index.Insert(library.Track{Artist: "A", Genre: "Blues", Title: "T3", Path: "/3", Hash: "h3"})

	m.browse.Genres.Genres = gentui.DeriveGenres(m.library.Index.Tracks())
	original := make([]string, len(m.browse.Genres.Genres))
	copy(original, m.browse.Genres.Genres)

	m.browse.Genres.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.Genres = m.browse.Genres.RecomputeGenresSort()

	if len(m.browse.Genres.Genres) > 1 && m.browse.Genres.Genres[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.Genres.SortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.Genres.SortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.Genres.Genres) {
			t.Fatalf("genres list too short: got %d, want %d", len(m.browse.Genres.Genres), len(original))
		}
		if m.browse.Genres.Genres[i] != want {
			t.Errorf("genres[%d] = %q, want %q", i, m.browse.Genres.Genres[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesFavoritesTracks(t *testing.T) {
	m := Model{}
	m.ActiveView = viewFavorites
	m.library.Index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "ha"})
	m.library.Index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "hb"})
	m.library.Index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "hc"})

	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	allTracks := m.library.Index.Tracks()
	m.favorites.Tracks = make([]library.Track, len(allTracks))
	copy(m.favorites.Tracks, allTracks)

	originalOrder := make([]string, len(m.favorites.Tracks))
	for i, tr := range m.favorites.Tracks {
		originalOrder[i] = tr.Hash
	}

	m.favorites.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.favorites = m.favorites.RecomputeFavoritesSort()

	if m.favorites.Tracks[0].Hash == originalOrder[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	for m.favorites.SortState.Field != sorting.SortDefault {
		result, _, _ := m.handleSortCycle()
		m = result.(Model)
	}

	for i, wantHash := range originalOrder {
		if i >= len(m.favorites.Tracks) {
			t.Fatalf("favorites tracks too short: got %d, want %d", len(m.favorites.Tracks), len(originalOrder))
		}
		if m.favorites.Tracks[i].Hash != wantHash {
			t.Errorf("favorites.tracks[%d].Hash = %q, want %q", i, m.favorites.Tracks[i].Hash, wantHash)
		}
	}
}

func TestSortCycleRecentlyAddedHandled(t *testing.T) {
	m := Model{}
	m.library.Mode = libtui.LibraryModeRecent
	m.ActiveView = viewLibrary
	m.library.Index.Insert(library.Track{Artist: "A", Title: "T1", Path: "/1", Hash: "h1"})

	m.library.SortState = sorting.SortState{Field: sorting.SortDefault, Dir: sorting.SortAsc}

	result, _, handled := m.handleSortCycle()
	if !handled {
		t.Fatal("handleSortCycle should handle recently added view")
	}
	m = result.(Model)

	if m.library.SortState.Field != sorting.SortDateAdded {
		t.Fatalf("expected SortDateAdded after cycle from default, got %v", m.library.SortState.Field)
	}
	if m.library.Cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", m.library.Cursor)
	}
	if m.library.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset reset to 0, got %d", m.library.ScrollOffset)
	}
}
