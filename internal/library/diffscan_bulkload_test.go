package library

import (
	"fmt"
	"testing"
)

func TestBulkLoad_SortsCorrectly(t *testing.T) {
	tracks := []Track{
		{Artist: "Zappa", Title: "Bobby Brown", Path: "/z.flac", Hash: "h1"},
		{Artist: "Aphex Twin", Title: "Xtal", Path: "/x.flac", Hash: "h2"},
		{Artist: "Aphex Twin", Title: "Actium", Path: "/a.flac", Hash: "h3"},
		{Artist: unknownArtist, Title: "Unknown", Path: "/u.flac", Hash: "h4"},
		{Artist: "Miles Davis", Title: "So What", Path: "/m.flac", Hash: "h5"},
	}

	var insertIdx LibraryIndex
	for _, tr := range tracks {
		insertIdx.Insert(tr)
	}

	var bulkIdx LibraryIndex
	bulkIdx.BulkLoad(tracks)

	got := bulkIdx.Tracks()
	want := insertIdx.Tracks()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Hash != w.Hash {
			t.Errorf("position %d: got %q, want %q", i, got[i].Hash, w.Hash)
		}
	}
}

func TestBulkLoad_ReplacesExistingContent(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "Old", Title: "Old", Hash: "old", Path: "/old.flac"})

	fresh := []Track{
		{Artist: "New", Title: "New", Hash: "new", Path: "/new.flac"},
	}
	idx.BulkLoad(fresh)

	if idx.Len() != 1 {
		t.Fatalf("Len: got %d, want 1", idx.Len())
	}
	if idx.Tracks()[0].Hash != "new" {
		t.Errorf("expected new track, got %q", idx.Tracks()[0].Hash)
	}
}

func TestBulkLoad_DuplicateHashMatchesInsertSemantics(t *testing.T) {
	tracks := []Track{
		{Hash: "h1", Artist: "B", Title: "old", Path: "/old.flac"},
		{Hash: "h2", Artist: "C", Title: "stable", Path: "/stable.flac"},
		{Hash: "h1", Artist: "A", Title: "new", Path: "/new.flac"},
	}

	var insertIdx LibraryIndex
	for _, tr := range tracks {
		insertIdx.Insert(tr)
	}

	var bulkIdx LibraryIndex
	bulkIdx.BulkLoad(tracks)

	if bulkIdx.Len() != insertIdx.Len() {
		t.Fatalf("len: got %d, want %d", bulkIdx.Len(), insertIdx.Len())
	}
	got, ok := bulkIdx.FindByHash("h1")
	if !ok {
		t.Fatal("expected h1 to be indexed")
	}
	if got.Path != "/new.flac" {
		t.Fatalf("h1 should keep last duplicate, got path %q", got.Path)
	}

	gotTracks := bulkIdx.Tracks()
	wantTracks := insertIdx.Tracks()
	for i := range wantTracks {
		if gotTracks[i].Hash != wantTracks[i].Hash || gotTracks[i].Path != wantTracks[i].Path {
			t.Fatalf("position %d mismatch: got %+v, want %+v", i, gotTracks[i], wantTracks[i])
		}
	}
}

func BenchmarkBulkLoad(b *testing.B) {
	const n = 50_000
	tracks := make([]Track, n)
	for i := range n {
		tracks[i] = Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/track%05d.flac", i),
		}
	}
	b.ResetTimer()
	for b.Loop() {
		var idx LibraryIndex
		idx.BulkLoad(tracks)
	}
}
