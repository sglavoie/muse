package library

import "testing"

func TestIncrementPlayCount_Found(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1", PlayCount: 5})
	idx.Insert(Track{Hash: "bbb", Artist: "B", Title: "t2", PlayCount: 0})

	now := int64(1700000000)
	if !idx.IncrementPlayCount("bbb", now) {
		t.Fatal("IncrementPlayCount: expected true for existing hash")
	}

	got, ok := idx.FindByHash("bbb")
	if !ok {
		t.Fatal("track not found after IncrementPlayCount")
	}
	if got.PlayCount != 1 {
		t.Errorf("expected PlayCount=1, got %d", got.PlayCount)
	}
	if got.LastPlayedAt != now {
		t.Errorf("expected LastPlayedAt=%d, got %d", now, got.LastPlayedAt)
	}

	if !idx.IncrementPlayCount("bbb", now+60) {
		t.Fatal("second IncrementPlayCount: expected true")
	}
	got, _ = idx.FindByHash("bbb")
	if got.PlayCount != 2 {
		t.Errorf("expected PlayCount=2, got %d", got.PlayCount)
	}
	if got.LastPlayedAt != now+60 {
		t.Errorf("expected LastPlayedAt=%d, got %d", now+60, got.LastPlayedAt)
	}
}

func TestIncrementPlayCount_BackwardsClockDoesNotRewindTimestamp(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1"})

	now := int64(1700000000)
	if !idx.IncrementPlayCount("aaa", now) {
		t.Fatal("first IncrementPlayCount: expected true")
	}

	// Simulate an NTP correction rewinding the clock by 5 minutes between
	// plays. The in-memory index must not rewind LastPlayedAt, otherwise the
	// History view reorders rows incorrectly until the next reload.
	if !idx.IncrementPlayCount("aaa", now-300) {
		t.Fatal("second IncrementPlayCount: expected true")
	}
	got, _ := idx.FindByHash("aaa")
	if got.PlayCount != 2 {
		t.Errorf("expected PlayCount=2, got %d", got.PlayCount)
	}
	if got.LastPlayedAt != now {
		t.Errorf("expected LastPlayedAt=%d (clamped), got %d", now, got.LastPlayedAt)
	}
}

func TestIncrementPlayCount_NotFound(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "A", Title: "t1"})

	if idx.IncrementPlayCount("zzz", 1700000000) {
		t.Error("IncrementPlayCount: expected false for non-existing hash")
	}
}

func TestIncrementPlayCount_EmptyIndex(t *testing.T) {
	var idx LibraryIndex
	if idx.IncrementPlayCount("aaa", 1700000000) {
		t.Error("IncrementPlayCount on empty index: expected false")
	}
}

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

	idx.UpsertByPath("/a.flac", Track{Artist: "Arca", Title: "t1", Path: "/a.flac"})

	got := idx.Tracks()
	if got[0].Artist != "Arca" {
		t.Errorf("expected Arca first after upsert, got %q", got[0].Artist)
	}
}

func TestUpdateMetadata_UpdatesAndResorts(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "aaa", Artist: "Zappa", Title: "Bobby Brown"})
	idx.Insert(Track{Hash: "bbb", Artist: "Miles", Title: "So What"})

	ok := idx.UpdateMetadata("aaa", "Piel", "Arca", "Kick", "")
	if !ok {
		t.Fatal("UpdateMetadata: expected true for existing hash")
	}

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

	ok := idx.UpdateMetadata("zzz", "X", "Y", "Z", "")
	if ok {
		t.Error("UpdateMetadata: expected false for non-existing hash")
	}
	if idx.Len() != 1 {
		t.Errorf("index length changed unexpectedly: %d", idx.Len())
	}
}

func TestUpdateMetadata_EmptyHashIgnored(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "A", Title: "original", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h1", Artist: "B", Title: "kept", Path: "/b.flac"})

	if idx.UpdateMetadata("", "changed", "Changed", "Changed", "Changed") {
		t.Fatal("UpdateMetadata should ignore empty hash")
	}

	got, ok := idx.FindByPath("/a.flac")
	if !ok {
		t.Fatal("empty-hash track should still be present")
	}
	if got.Artist != "A" || got.Title != "original" || got.Album != "" || got.Genre != "" {
		t.Fatalf("empty-hash track changed unexpectedly: %+v", got)
	}
	if _, ok := idx.FindByHash("h1"); !ok {
		t.Fatal("non-empty hash should remain indexed")
	}
}

func TestInsert_DuplicateHashReplacesExisting(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "OldTitle", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})

	if idx.Len() != 2 {
		t.Fatalf("expected 2 tracks, got %d", idx.Len())
	}

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

	idx.Insert(Track{Hash: "h1", Artist: "Alice", Title: "Song"})

	if idx.Len() != 2 {
		t.Fatalf("after re-insert with new sort key: expected 2, got %d", idx.Len())
	}

	got := idx.Tracks()
	if got[0].Artist != "Alice" {
		t.Errorf("position 0: expected Alice, got %q", got[0].Artist)
	}
	if got[1].Artist != "Miles" {
		t.Errorf("position 1: expected Miles, got %q", got[1].Artist)
	}

	if _, ok := idx.FindByHash("h1"); !ok {
		t.Error("h1 not findable after re-insert")
	}
	if _, ok := idx.FindByHash("h2"); !ok {
		t.Error("h2 not findable after re-insert")
	}
}

func TestUpsertByPath_DuplicateHashDeduplicates(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Path: "/a.flac"})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Path: "/b.flac"})

	// Upsert /a.flac with a hash that already exists at /b.flac.
	// Without dedup, /b.flac's hash entry gets overwritten and it becomes
	// unfindable by hash, orphaning it in the index.
	idx.UpsertByPath("/a.flac", Track{Hash: "h2", Artist: "A", Title: "t1-new", Path: "/a.flac"})

	// The duplicate should have been removed, leaving only the upserted track.
	if idx.Len() != 1 {
		t.Fatalf("expected 1 track after upsert with duplicate hash, got %d", idx.Len())
	}

	found, ok := idx.FindByHash("h2")
	if !ok {
		t.Fatal("h2 not findable after upsert")
	}
	if found.Path != "/a.flac" {
		t.Errorf("expected path /a.flac, got %q", found.Path)
	}
	if found.Title != "t1-new" {
		t.Errorf("expected title t1-new, got %q", found.Title)
	}
}

func TestUpdateDuration_Found(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1", Duration: 0})
	idx.Insert(Track{Hash: "h2", Artist: "B", Title: "t2", Duration: 100})

	if !idx.UpdateDuration("h1", 42.5) {
		t.Fatal("UpdateDuration: expected true for existing hash")
	}
	got, ok := idx.FindByHash("h1")
	if !ok {
		t.Fatal("track not found after UpdateDuration")
	}
	if got.Duration != 42.5 {
		t.Errorf("expected Duration=42.5, got %f", got.Duration)
	}
	got2, _ := idx.FindByHash("h2")
	if got2.Duration != 100 {
		t.Errorf("h2 Duration changed unexpectedly: %f", got2.Duration)
	}
}

func TestUpdateDuration_NotFound(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "A", Title: "t1"})

	if idx.UpdateDuration("nonexistent", 99) {
		t.Error("UpdateDuration: expected false for non-existing hash")
	}
}

func TestUpdateDuration_EmptyIndex(t *testing.T) {
	var idx LibraryIndex
	if idx.UpdateDuration("h1", 42) {
		t.Error("UpdateDuration on empty index: expected false")
	}
}

func TestUpdateDuration_DoesNotChangeSortOrder(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Hash: "h1", Artist: "B", Title: "t1", Duration: 0})
	idx.Insert(Track{Hash: "h2", Artist: "A", Title: "t2", Duration: 100})

	idx.UpdateDuration("h1", 999)
	got := idx.Tracks()
	if got[0].Artist != "A" || got[1].Artist != "B" {
		t.Errorf("UpdateDuration changed sort order: %v", got)
	}
}
