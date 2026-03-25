package library

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestIndex is the entry point matched by -run TestIndex; subtests cover all
// index behaviours.
func TestIndex(t *testing.T) {
	t.Run("IncrementalInsertion", testIndexIncrementalInsertion)
	t.Run("ArtistThenTitleSortStability", testIndexArtistThenTitleSortStability)
	t.Run("UnknownArtistBottomSort", testIndexUnknownArtistBottomSort)
}

func testIndexIncrementalInsertion(t *testing.T) {
	var idx LibraryIndex
	toInsert := []Track{
		{Artist: "Z", Title: "z1"},
		{Artist: "A", Title: "a1"},
		{Artist: "M", Title: "m1"},
	}
	for i, tr := range toInsert {
		idx.Insert(tr)
		if idx.Len() != i+1 {
			t.Fatalf("after %d inserts: got Len()=%d, want %d", i+1, idx.Len(), i+1)
		}
	}
	// Tracks must be in sorted order after each incremental insert.
	got := idx.Tracks()
	if got[0].Artist != "A" || got[1].Artist != "M" || got[2].Artist != "Z" {
		t.Errorf("unexpected order after incremental inserts: %v", got)
	}
}

func testIndexArtistThenTitleSortStability(t *testing.T) {
	var idx LibraryIndex
	tracks := []Track{
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}
	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	got := idx.Tracks()
	if len(got) != len(want) {
		t.Fatalf("got %d tracks, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func testIndexUnknownArtistBottomSort(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: unknownArtist, Title: "zzz"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})
	idx.Insert(Track{Artist: unknownArtist, Title: "aaa"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})

	got := idx.Tracks()
	// All named artists must precede Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Unknown Artist entries must themselves be title-sorted.
	if got[2].Title != "aaa" || got[3].Title != "zzz" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
	}
}

// --- less function ---

func TestLess_NamedBeforeUnknown(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	b := Track{Artist: unknownArtist, Title: "abc"}
	if !less(a, b) {
		t.Error("named artist must sort before Unknown Artist")
	}
	if less(b, a) {
		t.Error("Unknown Artist must not sort before named artist")
	}
}

func TestLess_SameArtistByTitle(t *testing.T) {
	a := Track{Artist: "Aphex Twin", Title: "Actium"}
	b := Track{Artist: "Aphex Twin", Title: "Xtal"}
	if !less(a, b) {
		t.Error("same artist: earlier title must sort first")
	}
	if less(b, a) {
		t.Error("same artist: later title must not sort first")
	}
}

func TestLess_DifferentArtists(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	b := Track{Artist: "Zappa", Title: "Bobby Brown"}
	if !less(a, b) {
		t.Error("Arca must sort before Zappa")
	}
}

func TestLess_BothUnknownByTitle(t *testing.T) {
	a := Track{Artist: unknownArtist, Title: "aaa"}
	b := Track{Artist: unknownArtist, Title: "zzz"}
	if !less(a, b) {
		t.Error("same Unknown Artist: earlier title must sort first")
	}
}

func TestLess_IdenticalTracks(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	if less(a, a) {
		t.Error("identical tracks: less must return false")
	}
}

// --- RemoveByPath ---

func TestRemoveByPath_Exists(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Artist: "C", Title: "t3", Path: "/c.flac"})

	if !idx.RemoveByPath("/b.flac") {
		t.Fatal("RemoveByPath: expected true for existing path")
	}
	if idx.Len() != 2 {
		t.Fatalf("expected 2 tracks after remove, got %d", idx.Len())
	}
	for _, tr := range idx.Tracks() {
		if tr.Path == "/b.flac" {
			t.Error("removed track still present")
		}
	}
}

func TestRemoveByPath_NotExists(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "t1", Path: "/a.flac"})

	if idx.RemoveByPath("/nonexistent.flac") {
		t.Error("RemoveByPath: expected false for non-existing path")
	}
	if idx.Len() != 1 {
		t.Errorf("expected 1 track, got %d", idx.Len())
	}
}

func TestRemoveByPath_EmptyIndex(t *testing.T) {
	var idx LibraryIndex
	if idx.RemoveByPath("/a.flac") {
		t.Error("RemoveByPath on empty index: expected false")
	}
}

// --- UpsertByPath ---

func TestUpsertByPath_InsertNew(t *testing.T) {
	var idx LibraryIndex
	idx.UpsertByPath("/a.flac", Track{Artist: "A", Title: "t1", Path: "/a.flac"})
	if idx.Len() != 1 {
		t.Fatalf("expected 1 track, got %d", idx.Len())
	}
}

func TestUpsertByPath_ReplaceExisting(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "OldTitle", Path: "/a.flac"})
	idx.Insert(Track{Artist: "B", Title: "t2", Path: "/b.flac"})

	idx.UpsertByPath("/a.flac", Track{Artist: "A", Title: "NewTitle", Path: "/a.flac"})

	if idx.Len() != 2 {
		t.Fatalf("expected 2 tracks after upsert, got %d", idx.Len())
	}
	// Verify the old entry is gone and new one exists.
	for _, tr := range idx.Tracks() {
		if tr.Path == "/a.flac" && tr.Title != "NewTitle" {
			t.Errorf("upsert did not replace: got title %q", tr.Title)
		}
	}
}

func TestUpsertByPath_ChangedArtistResortsCorrectly(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "Zappa", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Artist: "Miles", Title: "t2", Path: "/b.flac"})

	// Change artist from Zappa to Arca — should now sort first.
	idx.UpsertByPath("/a.flac", Track{Artist: "Arca", Title: "t1", Path: "/a.flac"})

	got := idx.Tracks()
	if got[0].Artist != "Arca" {
		t.Errorf("expected Arca first after upsert, got %q", got[0].Artist)
	}
}

// BenchmarkIndex measures insertion of 50,000 tracks and reports per-op
// allocations. Run with -bench BenchmarkIndex.
func BenchmarkIndex(b *testing.B) {
	for b.Loop() {
		var idx LibraryIndex
		for i := range 50_000 {
			idx.Insert(Track{
				Artist: fmt.Sprintf("Artist%05d", i%1000),
				Title:  fmt.Sprintf("Track%05d", i),
				Album:  fmt.Sprintf("Album%05d", i%200),
				Hash:   fmt.Sprintf("%064x", i),
				Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
			})
		}
	}
}

func TestLibraryIndex_InsertSortedOrder(t *testing.T) {
	var idx LibraryIndex

	tracks := []Track{
		{Artist: "Zappa", Title: "Bobby Brown"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Aphex Twin", Title: "Flim"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}

	got := idx.Tracks()
	if len(got) != len(tracks) {
		t.Fatalf("expected %d tracks, got %d", len(tracks), len(got))
	}

	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func TestLibraryIndex_UnknownArtistSortsLast(t *testing.T) {
	var idx LibraryIndex

	idx.Insert(Track{Artist: unknownArtist, Title: "some-file"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})
	idx.Insert(Track{Artist: unknownArtist, Title: "another-file"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})

	got := idx.Tracks()
	// All named artists must come before Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Verify Unknown Artist entries are themselves title-sorted.
	if got[2].Title != "another-file" || got[3].Title != "some-file" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
	}
}

func TestLibraryIndex_Sub100msLatency(t *testing.T) {
	const n = 10_001

	var idx LibraryIndex
	for i := range n {
		idx.Insert(Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Album:  fmt.Sprintf("Album%05d", i%200),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
		})
	}

	start := time.Now()
	tracks := idx.Tracks()
	for range tracks {
	}
	elapsed := time.Since(start)

	t.Logf("Tracks() access + iteration over %d tracks: %v", n, elapsed)

	const limit = 100 * time.Millisecond
	if elapsed >= limit {
		t.Errorf("access + iteration took %v, want < %v", elapsed, limit)
	}
}

// --- FindByHash ---

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

// --- UpdateMetadata ---

func TestUpdateMetadata_UpdatesAndResorts(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "Zappa", Title: "Bobby Brown"})
	idx.Insert(Track{Hash: "bbb", Artist: "Miles", Title: "So What"})

	ok := idx.UpdateMetadata("aaa", "Piel", "Arca", "Kick")
	if !ok {
		t.Fatal("UpdateMetadata: expected true for existing hash")
	}

	// Arca should now sort before Miles.
	got := idx.Tracks()
	if len(got) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(got))
	}
	if got[0].Artist != "Arca" || got[0].Title != "Piel" || got[0].Album != "Kick" {
		t.Errorf("position 0: got %+v, want Arca/Piel/Kick", got[0])
	}
	if got[0].Hash != "aaa" {
		t.Errorf("hash should be preserved: got %q", got[0].Hash)
	}
	if got[1].Artist != "Miles" {
		t.Errorf("position 1: got artist %q, want Miles", got[1].Artist)
	}
}

func TestUpdateMetadata_NotFound(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1"})

	ok := idx.UpdateMetadata("zzz", "X", "Y", "Z")
	if ok {
		t.Error("UpdateMetadata: expected false for non-existing hash")
	}
	if idx.Len() != 1 {
		t.Errorf("index length changed unexpectedly: %d", idx.Len())
	}
}

// --- Insert dedup ---

func TestInsert_DuplicateHashReplacesExisting(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "OldTitle", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})

	if idx.Len() != 2 {
		t.Fatalf("expected 2 tracks, got %d", idx.Len())
	}

	// Re-insert h1 with updated metadata — should replace, not duplicate.
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "NewTitle", Path: "/a.flac"})

	if idx.Len() != 2 {
		t.Fatalf("after re-insert: expected 2 tracks, got %d", idx.Len())
	}

	found, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found after re-insert")
	}
	if found.Title != "NewTitle" {
		t.Errorf("expected Title=NewTitle, got %q", found.Title)
	}

	// Verify both hashes are findable.
	if _, ok := idx.FindByHash("h2"); !ok {
		t.Error("h2 not found after re-insert of h1")
	}
}

func TestInsert_EmptyHashDoesNotDedup(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "t1"})
	idx.Insert(Track{Artist: "B", Title: "t2"})

	if idx.Len() != 2 {
		t.Errorf("expected 2 tracks with empty hash, got %d", idx.Len())
	}
}

func TestInsert_DuplicateHashReordersCorrectly(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "Zappa", Title: "Bobby Brown"})
	idx.Insert(Track{Hash: "h2", Artist: "Miles", Title: "So What"})

	if idx.Len() != 2 {
		t.Fatalf("expected 2 tracks, got %d", idx.Len())
	}

	// Re-insert h1 with a different artist that changes sort position.
	idx.Insert(Track{Hash: "h1", Artist: "Alice", Title: "Song"})

	if idx.Len() != 2 {
		t.Fatalf("after re-insert with new sort key: expected 2, got %d", idx.Len())
	}

	got := idx.Tracks()
	// Alice < Miles in sort order.
	if got[0].Artist != "Alice" {
		t.Errorf("position 0: expected Alice, got %q", got[0].Artist)
	}
	if got[1].Artist != "Miles" {
		t.Errorf("position 1: expected Miles, got %q", got[1].Artist)
	}

	// Verify both hashes are findable at correct positions.
	if _, ok := idx.FindByHash("h1"); !ok {
		t.Error("h1 not findable after re-insert")
	}
	if _, ok := idx.FindByHash("h2"); !ok {
		t.Error("h2 not findable after re-insert")
	}
}

// --- Hash map consistency ---

func TestHashMapConsistency(t *testing.T) {
	var idx LibraryIndex

	// Insert: hash map must reflect new entry.
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})
	idx.Insert(Track{Hash: "h3", Artist: "C", Title: "t3", Path: "/c.flac"})

	for _, h := range []string{"h1", "h2", "h3"} {
		if _, ok := idx.FindByHash(h); !ok {
			t.Errorf("after Insert: hash %q not found", h)
		}
	}

	// Remove: removed hash must no longer be found.
	idx.RemoveByPath("/b.flac")
	if _, ok := idx.FindByHash("h2"); ok {
		t.Error("after RemoveByPath: removed hash h2 still found")
	}
	if _, ok := idx.FindByHash("h1"); !ok {
		t.Error("after RemoveByPath: h1 should still be found")
	}

	// UpdateMetadata: updated hash still findable, fields updated.
	idx.UpdateMetadata("h1", "NewTitle", "NewArtist", "NewAlbum")
	got, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("after UpdateMetadata: h1 not found")
	}
	if got.Title != "NewTitle" || got.Artist != "NewArtist" || got.Album != "NewAlbum" {
		t.Errorf("after UpdateMetadata: got %+v", got)
	}

	// BulkLoad: map rebuilt from scratch.
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
	// Old hashes must be gone.
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

// TestHashMapConsistencyAfterInterleavedOps verifies that after a sequence of
// Insert, Remove, Upsert, and UpdateMetadata calls the byHash map correctly
// points to each track's actual position in the slice.
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

	idx.UpdateMetadata("h3", "t3-new", "AA", "AlbumX")
	assertConsistent(t, &idx)

	idx.RemoveByPath("/d.flac")
	assertConsistent(t, &idx)
}

func TestLibraryIndex_MemoryUnder25MB(t *testing.T) {
	const n = 50_000

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	var idx LibraryIndex
	for i := range n {
		idx.Insert(Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Album:  fmt.Sprintf("Album%05d", i%200),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
		})
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	peakMB := float64(after.HeapInuse-before.HeapInuse) / (1024 * 1024)
	t.Logf("heap delta for %d tracks: %.2f MB", n, peakMB)

	const limitMB = 25.0
	if peakMB > limitMB {
		t.Errorf("peak heap delta %.2f MB exceeds %.0f MB limit", peakMB, limitMB)
	}

	if idx.Len() != n {
		t.Errorf("expected %d tracks, got %d", n, idx.Len())
	}
}
