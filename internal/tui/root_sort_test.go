package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
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

	v := LibraryView{
		index:     idx,
		sortState: sorting.SortState{Field: sorting.SortDefault, Dir: sorting.SortAsc},
	}

	v.sortState.Field = sorting.CycleSortField(v.sortState.Field, sorting.LibrarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != sorting.SortArtist {
		t.Fatalf("expected SortArtist, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Artist != "Alpha" {
		t.Errorf("first track by artist should be Alpha, got %q", v.sortedTracks[0].Artist)
	}

	v.sortState.Field = sorting.CycleSortField(v.sortState.Field, sorting.LibrarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != sorting.SortTitle {
		t.Fatalf("expected SortTitle, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Title != "Alpha" {
		t.Errorf("first track by title should be Alpha, got %q", v.sortedTracks[0].Title)
	}

	v.sortState.Field = sorting.CycleSortField(v.sortState.Field, sorting.LibrarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != sorting.SortAlbum {
		t.Fatalf("expected SortAlbum, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Album != "Blue" {
		t.Errorf("first track by album should be Blue, got %q", v.sortedTracks[0].Album)
	}

	v.sortState.Field = sorting.CycleSortField(v.sortState.Field, sorting.LibrarySortFields)
	v = v.recomputeSortedTracks()
	if v.sortState.Field != sorting.SortDuration {
		t.Fatalf("expected SortDuration, got %d", v.sortState.Field)
	}
	if v.sortedTracks[0].Duration != 100 {
		t.Errorf("first track by duration should be 100, got %f", v.sortedTracks[0].Duration)
	}

	v.sortState.Field = sorting.CycleSortField(v.sortState.Field, sorting.LibrarySortFields)
	if v.sortState.Field != sorting.SortDefault {
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
		sortState: sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc},
	}
	v = v.recomputeSortedTracks()

	if v.sortedTracks[0].Artist != "Alpha" {
		t.Errorf("asc first should be Alpha, got %q", v.sortedTracks[0].Artist)
	}
	if v.sortedTracks[2].Artist != "Charlie" {
		t.Errorf("asc last should be Charlie, got %q", v.sortedTracks[2].Artist)
	}

	v.sortState.Dir = sorting.SortDesc
	v = v.recomputeSortedTracks()
	if v.sortedTracks[0].Artist != "Charlie" {
		t.Errorf("desc first should be Charlie, got %q", v.sortedTracks[0].Artist)
	}
	if v.sortedTracks[2].Artist != "Alpha" {
		t.Errorf("desc last should be Alpha, got %q", v.sortedTracks[2].Artist)
	}
}

func TestSortResetOnViewSwitch(t *testing.T) {
	m := Model{
		activeView: viewLibrary,
		library: LibraryView{
			sortState: sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortDesc},
			sortedTracks: []library.Track{
				{Artist: "X"},
			},
		},
		queue: QueueView{
			sortState:    sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
			displayOrder: []int{1, 0},
		},
	}

	m = m.resetLibrarySort()
	if m.library.sortState.Field != sorting.SortDefault {
		t.Error("library sort should be reset")
	}
	if m.library.sortedTracks != nil {
		t.Error("library sortedTracks should be nil")
	}

	m = m.resetQueueSort()
	if m.queue.sortState.Field != sorting.SortDefault {
		t.Error("queue sort should be reset")
	}
	if m.queue.displayOrder != nil {
		t.Error("queue displayOrder should be nil")
	}
}

func TestResetAllSorts(t *testing.T) {
	m := Model{
		library:   LibraryView{sortState: sorting.SortState{Field: sorting.SortArtist}},
		queue:     QueueView{sortState: sorting.SortState{Field: sorting.SortTitle}},
		playlists: PlaylistsView{sortState: sorting.SortState{Field: sorting.SortName}},
		browse: BrowseView{
			artists: ArtistsView{sortState: sorting.SortState{Field: sorting.SortName}},
			albums:  AlbumsView{sortState: sorting.SortState{Field: sorting.SortTitle}},
		},
	}

	m = m.resetAllSorts()

	if m.library.sortState.Field != sorting.SortDefault {
		t.Error("library sort not reset")
	}
	if m.queue.sortState.Field != sorting.SortDefault {
		t.Error("queue sort not reset")
	}
	if m.playlists.sortState.Field != sorting.SortDefault {
		t.Error("playlists sort not reset")
	}
	if m.browse.artists.sortState.Field != sorting.SortDefault {
		t.Error("artists sort not reset")
	}
	if m.browse.albums.sortState.Field != sorting.SortDefault {
		t.Error("albums sort not reset")
	}
}

func TestSortKeybindingsRegistered(t *testing.T) {
	reg := newRegistry()

	tests := []struct {
		key  string
		want action
	}{
		{"ctrl+s", actionSortCycle},
		{"shift+up", actionSortAsc},
		{"shift+down", actionSortDesc},
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

	contexts := []struct {
		name string
		ctx  bindingContext
	}{
		{"library", contextLibrary},
		{"library", contextLibrary},
		{"queue", contextQueue},
		{"playlists", contextPlaylists},
		{"artists", contextArtists},
		{"albums", contextAlbums},
		{"favorites", contextFavorites},
		{"genres", contextGenres},
	}

	for _, tc := range contexts {
		got := reg.resolve(tc.ctx, "ctrl+d")
		if got != actionHalfPageDown {
			t.Errorf("resolve(%s, ctrl+d) = %v, want actionHalfPageDown", tc.name, got)
		}
	}
}

func TestSortCycleToDefaultRederivesArtists(t *testing.T) {
	m := Model{}
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.library.index.Insert(library.Track{Artist: "B", Title: "T1", Path: "/b", Hash: "hb"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "T2", Path: "/a", Hash: "ha"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "T3", Path: "/c", Hash: "hc"})

	m.browse.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	original := make([]string, len(m.browse.artists.artists))
	copy(original, m.browse.artists.artists)

	m.browse.artists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.artists = m.browse.artists.recomputeArtistsSort()

	if len(m.browse.artists.artists) > 1 && m.browse.artists.artists[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.artists.sortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.artists.sortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.artists.artists) {
			t.Fatalf("artists list too short: got %d, want %d", len(m.browse.artists.artists), len(original))
		}
		if m.browse.artists.artists[i] != want {
			t.Errorf("artists[%d] = %q, want %q", i, m.browse.artists.artists[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesAlbums(t *testing.T) {
	m := Model{}
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.library.index.Insert(library.Track{Artist: "A", Album: "Zebra", Title: "T1", Path: "/1", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "A", Album: "Alpha", Title: "T2", Path: "/2", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "A", Album: "Mid", Title: "T3", Path: "/3", Hash: "h3"})

	m.browse.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
	original := make([]string, len(m.browse.albums.albums))
	copy(original, m.browse.albums.albums)

	m.browse.albums.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.albums = m.browse.albums.recomputeAlbumsSort()

	if len(m.browse.albums.albums) > 1 && m.browse.albums.albums[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.albums.sortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.albums.sortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.albums.albums) {
			t.Fatalf("albums list too short: got %d, want %d", len(m.browse.albums.albums), len(original))
		}
		if m.browse.albums.albums[i] != want {
			t.Errorf("albums[%d] = %q, want %q", i, m.browse.albums.albums[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesGenres(t *testing.T) {
	m := Model{}
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeGenres
	m.library.index.Insert(library.Track{Artist: "A", Genre: "Rock", Title: "T1", Path: "/1", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "A", Genre: "Jazz", Title: "T2", Path: "/2", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "A", Genre: "Blues", Title: "T3", Path: "/3", Hash: "h3"})

	m.browse.genres.genres = gentui.DeriveGenres(m.library.index.Tracks())
	original := make([]string, len(m.browse.genres.genres))
	copy(original, m.browse.genres.genres)

	m.browse.genres.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.genres = m.browse.genres.recomputeGenresSort()

	if len(m.browse.genres.genres) > 1 && m.browse.genres.genres[0] == original[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.browse.genres.sortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault after cycle, got %v", m.browse.genres.sortState.Field)
	}

	for i, want := range original {
		if i >= len(m.browse.genres.genres) {
			t.Fatalf("genres list too short: got %d, want %d", len(m.browse.genres.genres), len(original))
		}
		if m.browse.genres.genres[i] != want {
			t.Errorf("genres[%d] = %q, want %q", i, m.browse.genres.genres[i], want)
		}
	}
}

func TestSortCycleToDefaultRederivesFavoritesTracks(t *testing.T) {
	m := Model{}
	m.activeView = viewFavorites
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "ha"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "hb"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "hc"})

	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	allTracks := m.library.index.Tracks()
	m.favorites.tracks = make([]library.Track, len(allTracks))
	copy(m.favorites.tracks, allTracks)

	originalOrder := make([]string, len(m.favorites.tracks))
	for i, tr := range m.favorites.tracks {
		originalOrder[i] = tr.Hash
	}

	m.favorites.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.favorites = m.favorites.recomputeFavoritesSort()

	if m.favorites.tracks[0].Hash == originalOrder[0] {
		t.Fatal("sort did not change the order — test is invalid")
	}

	for m.favorites.sortState.Field != sorting.SortDefault {
		result, _, _ := m.handleSortCycle()
		m = result.(Model)
	}

	for i, wantHash := range originalOrder {
		if i >= len(m.favorites.tracks) {
			t.Fatalf("favorites tracks too short: got %d, want %d", len(m.favorites.tracks), len(originalOrder))
		}
		if m.favorites.tracks[i].Hash != wantHash {
			t.Errorf("favorites.tracks[%d].Hash = %q, want %q", i, m.favorites.tracks[i].Hash, wantHash)
		}
	}
}

func TestSortCycleRecentlyAddedHandled(t *testing.T) {
	m := Model{}
	m.library.mode = LibraryModeRecent
	m.activeView = viewLibrary
	m.library.index.Insert(library.Track{Artist: "A", Title: "T1", Path: "/1", Hash: "h1"})

	m.library.sortState = sorting.SortState{Field: sorting.SortDefault, Dir: sorting.SortAsc}

	result, _, handled := m.handleSortCycle()
	if !handled {
		t.Fatal("handleSortCycle should handle recently added view")
	}
	m = result.(Model)

	if m.library.sortState.Field != sorting.SortDateAdded {
		t.Fatalf("expected SortDateAdded after cycle from default, got %v", m.library.sortState.Field)
	}
	if m.library.cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", m.library.cursor)
	}
	if m.library.scrollOffset != 0 {
		t.Errorf("expected scrollOffset reset to 0, got %d", m.library.scrollOffset)
	}
}
