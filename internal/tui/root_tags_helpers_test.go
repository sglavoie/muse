package tui

import (
	"context"
	"sort"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestSelectedTrackHashes_Queue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1"},
		{Hash: "h2", Selected: true},
		{Hash: "h3"},
		{Hash: "h4", Selected: true},
	}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d", len(got))
	}
	set := map[string]struct{}{}
	for _, h := range got {
		set[h] = struct{}{}
	}
	for _, want := range []string{"h2", "h4"} {
		if _, ok := set[want]; !ok {
			t.Errorf("expected hash %q in result", want)
		}
	}
}

func TestSelectedTrackHashes_Artists(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewArtists
	m.artists.selectedHashes = map[string]struct{}{
		"ha1": {},
		"ha2": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d", len(got))
	}
}

func TestSelectedTrackHashes_Albums(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewAlbums
	m.albums.selectedHashes = map[string]struct{}{
		"hab1": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 1 {
		t.Fatalf("expected 1 selected hash, got %d", len(got))
	}
	if got[0] != "hab1" {
		t.Errorf("expected hash %q, got %q", "hab1", got[0])
	}
}

func TestSelectedTrackHashes_Genres(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewGenres
	m.genres.selectedHashes = map[string]struct{}{
		"hg1": {},
		"hg2": {},
		"hg3": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 3 {
		t.Fatalf("expected 3 selected hashes, got %d", len(got))
	}
}

func TestSelectedTrackHashes_Tags(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.selectedHashes = map[string]struct{}{
		"ht1": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 1 {
		t.Fatalf("expected 1 selected hash, got %d", len(got))
	}
}

func TestSelectedTrackHashes_Library(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewLibrary
	m.library.selectedHashes = map[string]struct{}{
		"h1": {},
		"h2": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d", len(got))
	}
}

func TestSelectedTrackHashes_EmptySelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1"},
		{Hash: "h2"},
	}

	got := m.selectedTrackHashes()
	if len(got) != 0 {
		t.Fatalf("expected 0 selected hashes from unselected queue, got %d", len(got))
	}
}

func TestTagPickerHashes_BulkFromQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Selected: true},
		{Hash: "h2"},
		{Hash: "h3", Selected: true},
	}

	hashes, isBulk := m.tagPickerHashes()
	if !isBulk {
		t.Error("expected isBulk=true for multi-selected queue entries")
	}
	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
}

func TestTagPickerHashes_SingleFocusedFromQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1"},
		{Hash: "h2"},
	}
	m.queue.cursor = 1

	hashes, isBulk := m.tagPickerHashes()
	if isBulk {
		t.Error("expected isBulk=false for single focused track")
	}
	if len(hashes) != 1 || hashes[0] != "h2" {
		t.Fatalf("expected [h2], got %v", hashes)
	}
}

func TestTagPickerHashes_BulkFromArtists(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewArtists
	m.artists.selectedHashes = map[string]struct{}{
		"ha1": {},
		"ha2": {},
	}
	m.artists.tracks = []library.Track{
		{Hash: "ha1", Artist: "A", Title: "T1"},
		{Hash: "ha2", Artist: "A", Title: "T2"},
	}

	hashes, isBulk := m.tagPickerHashes()
	if !isBulk {
		t.Error("expected isBulk=true for multi-selected artist tracks")
	}
	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
}

func TestSelectedTrackHashes_Playlists(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "hp1"},
		{ID: 2, Hash: "hp2"},
		{ID: 3, Hash: "hp3"},
	}
	m.playlists.selectedTrackIDs = map[int64]struct{}{1: {}, 3: {}}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d", len(got))
	}
	set := map[string]struct{}{}
	for _, h := range got {
		set[h] = struct{}{}
	}
	for _, want := range []string{"hp1", "hp3"} {
		if _, ok := set[want]; !ok {
			t.Errorf("expected hash %q in result", want)
		}
	}
}

func TestSelectedTrackHashes_FavoritesTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "hf1", Title: "Fav A"},
		{Hash: "hf2", Title: "Fav B"},
	}
	m.favorites.selectedItems = map[int]struct{}{0: {}, 1: {}}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes, got %d", len(got))
	}
}

func TestSelectedTrackHashes_FavoritesListLevelTracks(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "hf1", Title: "Fav A"},
		{Hash: "hf2", Title: "Fav B"},
	}
	m.favorites.selectedItems = map[int]struct{}{1: {}}

	got := m.selectedTrackHashes()
	if len(got) != 1 {
		t.Fatalf("expected 1 selected hash, got %d", len(got))
	}
	if got[0] != "hf2" {
		t.Errorf("expected hash 'hf2', got %q", got[0])
	}
}

func TestSelectedTrackHashes_Mixes(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewMixes
	m.mixes.level = 1
	m.mixes.trackView.selectedHashes = map[string]struct{}{
		"mx1": {},
		"mx2": {},
	}

	got := m.selectedTrackHashes()
	if len(got) != 2 {
		t.Fatalf("expected 2 selected hashes from mixes, got %d", len(got))
	}
	set := map[string]struct{}{}
	for _, h := range got {
		set[h] = struct{}{}
	}
	for _, want := range []string{"mx1", "mx2"} {
		if _, ok := set[want]; !ok {
			t.Errorf("expected hash %q in result", want)
		}
	}
}

func TestSelectedTrackHashes_MixesListLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewMixes
	// level 0 = list level, no track selection should be returned.
	m.mixes.level = 0

	got := m.selectedTrackHashes()
	if got != nil {
		t.Errorf("expected nil at mixes list level, got %v", got)
	}
}

func TestHashMapToSlice_Sorted(t *testing.T) {
	m := map[string]struct{}{
		"c": {},
		"a": {},
		"b": {},
	}
	got := hashMapToSlice(m)
	if !sort.StringsAreSorted(got) {
		t.Errorf("expected sorted output, got %v", got)
	}
}
