package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
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
