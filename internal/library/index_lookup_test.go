package library

import "testing"

func TestFindByHash_Found(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1"})
	idx.Insert(Track{Hash: "bbb", Artist: "B", Title: "t2", Album: "Al2"})
	idx.Insert(Track{Hash: "ccc", Artist: "C", Title: "t3"})

	got, ok := idx.FindByHash("bbb")
	if !ok {
		t.Fatal("FindByHash: expected true for existing hash")
	}
	if got.Artist != "B" || got.Title != "t2" || got.Album != "Al2" {
		t.Errorf("FindByHash returned wrong track: %+v", got)
	}
}

func TestFindByHash_NotFound(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1"})

	_, ok := idx.FindByHash("zzz")
	if ok {
		t.Error("FindByHash: expected false for non-existing hash")
	}
}

func TestFindByHash_EmptyIndex(t *testing.T) {
	var idx LibraryIndex
	_, ok := idx.FindByHash("aaa")
	if ok {
		t.Error("FindByHash on empty index: expected false")
	}
}

func TestHashMapConsistency(t *testing.T) {
	var idx LibraryIndex

	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Hash: "h3", Artist: "C", Title: "t3", Path: "/c.flac"})

	for _, h := range []string{"h1", "h2", "h3"} {
		if _, ok := idx.FindByHash(h); !ok {
			t.Errorf("after Insert: hash %q not found", h)
		}
	}

	idx.RemoveByPath("/b.flac")
	if _, ok := idx.FindByHash("h2"); ok {
		t.Error("after RemoveByPath: removed hash h2 still found")
	}
	if _, ok := idx.FindByHash("h1"); !ok {
		t.Error("after RemoveByPath: h1 should still be found")
	}

	idx.UpdateMetadata("h1", "NewTitle", "NewArtist", "NewAlbum", "")
	got, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("after UpdateMetadata: h1 not found")
	}
	if got.Title != "NewTitle" || got.Artist != "NewArtist" || got.Album != "NewAlbum" {
		t.Errorf("after UpdateMetadata: got %+v", got)
	}

	bulk := []Track{
		{Hash: "x1", Artist: "X", Title: "t1"},
		{Hash: "x2", Artist: "Y", Title: "t2"},
	}
	idx.BulkLoad(bulk)
	for _, h := range []string{"x1", "x2"} {
		if _, ok := idx.FindByHash(h); !ok {
			t.Errorf("after BulkLoad: hash %q not found", h)
		}
	}
	if _, ok := idx.FindByHash("h1"); ok {
		t.Error("after BulkLoad: stale hash h1 still found")
	}
}

func TestFindByHash_AfterRemove(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "a", Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Hash: "b", Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Hash: "c", Artist: "C", Title: "t3", Path: "/c.flac"})

	idx.RemoveByPath("/b.flac")

	if _, ok := idx.FindByHash("b"); ok {
		t.Error("removed hash b should not be found")
	}
	if _, ok := idx.FindByHash("a"); !ok {
		t.Error("hash a should still be found after removing b")
	}
	if _, ok := idx.FindByHash("c"); !ok {
		t.Error("hash c should still be found after removing b")
	}
}

func TestHashMapConsistencyAfterInterleavedOps(t *testing.T) {
	assertConsistent := func(t *testing.T, idx *LibraryIndex) {
		t.Helper()
		tracks := idx.Tracks()
		for i, tr := range tracks {
			got, ok := idx.FindByHash(tr.Hash)
			if !ok {
				t.Errorf("hash %q not found in map (track at slice[%d])", tr.Hash, i)
				continue
			}
			if got != tr {
				t.Errorf("hash %q: map points to wrong track %+v, want %+v", tr.Hash, got, tr)
			}
		}
	}

	var idx LibraryIndex

	idx.Insert(Track{Hash: "h1", Artist: "C", Title: "t1", Path: "/c.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "A", Title: "t2", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h3", Artist: "B", Title: "t3", Path: "/b.flac"})
	assertConsistent(t, &idx)

	idx.RemoveByPath("/a.flac")
	assertConsistent(t, &idx)

	idx.UpsertByPath("/c.flac", Track{Hash: "h1", Artist: "Z", Title: "t1-updated", Path: "/c.flac"})
	assertConsistent(t, &idx)

	idx.Insert(Track{Hash: "h4", Artist: "D", Title: "t4", Path: "/d.flac"})
	assertConsistent(t, &idx)

	idx.UpdateMetadata("h3", "t3-new", "AA", "AlbumX", "")
	assertConsistent(t, &idx)

	idx.RemoveByPath("/d.flac")
	assertConsistent(t, &idx)
}

func TestFindByPath_Found(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Hash: "h3", Artist: "C", Title: "t3", Path: "/c.flac"})

	got, ok := idx.FindByPath("/b.flac")
	if !ok {
		t.Fatal("FindByPath: expected true for existing path")
	}
	if got.Hash != "h2" || got.Artist != "B" {
		t.Errorf("FindByPath returned wrong track: %+v", got)
	}
}

func TestFindByPath_NotFound(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})

	_, ok := idx.FindByPath("/nonexistent.flac")
	if ok {
		t.Error("FindByPath: expected false for non-existing path")
	}
}

func TestFindByPath_EmptyIndex(t *testing.T) {
	var idx LibraryIndex
	_, ok := idx.FindByPath("/a.flac")
	if ok {
		t.Error("FindByPath on empty index: expected false")
	}
}

func TestBulkLoad_EmptyClearsIndex(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1"})
	idx.BulkLoad(nil)
	if idx.Len() != 0 {
		t.Errorf("BulkLoad(nil): expected 0 tracks, got %d", idx.Len())
	}
	if _, ok := idx.FindByHash("h1"); ok {
		t.Error("BulkLoad(nil): stale hash should not be found")
	}
}

func TestBulkLoad_DoesNotModifyInput(t *testing.T) {
	input := []Track{
		{Hash: "h2", Artist: "Zappa", Title: "t2"},
		{Hash: "h1", Artist: "Arca", Title: "t1"},
	}
	origFirst := input[0].Artist

	var idx LibraryIndex
	idx.BulkLoad(input)

	if input[0].Artist != origFirst {
		t.Error("BulkLoad modified the input slice")
	}
}

func TestFindByHash_EmptyHashIsNeverIndexed(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Hash: "h1", Artist: "C", Title: "t3", Path: "/c.flac"})

	if _, ok := idx.FindByHash(""); ok {
		t.Fatal("empty hash should not be indexed")
	}
	if idx.IncrementPlayCount("", 1700000000) {
		t.Fatal("IncrementPlayCount should ignore empty hash")
	}

	if !idx.RemoveByPath("/a.flac") {
		t.Fatal("RemoveByPath: expected true for existing empty-hash path")
	}
	if _, ok := idx.FindByHash(""); ok {
		t.Fatal("empty hash should remain unindexed after removing another empty-hash track")
	}

	found, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("non-empty hash should still be findable")
	}
	if found.Path != "/c.flac" {
		t.Fatalf("FindByHash(h1): got path %q, want /c.flac", found.Path)
	}
}

func TestBulkLoad_EmptyHashesRemainUnindexed(t *testing.T) {
	var idx LibraryIndex
	idx.BulkLoad([]Track{
		{Artist: "A", Title: "t1", Path: "/a.flac"},
		{Artist: "B", Title: "t2", Path: "/b.flac"},
		{Hash: "h1", Artist: "C", Title: "t3", Path: "/c.flac"},
	})

	if _, ok := idx.FindByHash(""); ok {
		t.Fatal("BulkLoad should not index empty hashes")
	}
	if idx.UpdateDuration("", 42) {
		t.Fatal("UpdateDuration should ignore empty hash")
	}

	found, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("BulkLoad should still index non-empty hashes")
	}
	if found.Path != "/c.flac" {
		t.Fatalf("FindByHash(h1): got path %q, want /c.flac", found.Path)
	}
}

func TestTracks_ReturnsSnapshot(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})

	tracks := idx.Tracks()
	tracks[0].Hash = "corrupted"
	tracks[0].Artist = "Z"

	found, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("FindByHash(h1): expected original track to remain indexed")
	}
	if found.Hash != "h1" || found.Artist != "A" {
		t.Fatalf("Tracks() mutation leaked into index: %+v", found)
	}

	fresh := idx.Tracks()
	if fresh[0].Hash != "h1" || fresh[0].Artist != "A" {
		t.Fatalf("fresh Tracks() returned mutated data: %+v", fresh[0])
	}
}
