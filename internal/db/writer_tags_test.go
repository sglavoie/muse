package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// openTagDB returns a migrated DB and a Writer wired to its context.
func openTagDB(t *testing.T) (*db.DB, *db.Writer) {
	t.Helper()
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(func() { w.Wait() })
	return d, w
}

// --- CreateTag ---

func TestCreateTag_Basic(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("workout", "#ff0000")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if id <= 0 {
		t.Errorf("id = %d, want > 0", id)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("got %d tags, want 1", len(tags))
	}
	if tags[0].Name != "workout" || tags[0].Color != "#ff0000" {
		t.Errorf("unexpected tag %+v", tags[0])
	}
}

func TestCreateTag_DuplicateNameErrors(t *testing.T) {
	_, w := openTagDB(t)

	if _, err := w.CreateTag("chill", ""); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	if _, err := w.CreateTag("chill", ""); err == nil {
		t.Error("expected error on duplicate name, got nil")
	}
}

func TestCreateTag_CaseInsensitiveDuplicateErrors(t *testing.T) {
	_, w := openTagDB(t)

	if _, err := w.CreateTag("Chill", ""); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	if _, err := w.CreateTag("chill", ""); err == nil {
		t.Error("expected error on case-insensitive duplicate, got nil")
	}
}

// --- RenameTag ---

func TestRenameTag(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("old", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.RenameTag(id, "new"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "new" {
		t.Errorf("got %+v, want name=new", tags)
	}
}

// --- UpdateTagColor ---

func TestUpdateTagColor(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("focus", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.UpdateTagColor(id, "#00ff00"); err != nil {
		t.Fatalf("UpdateTagColor: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Color != "#00ff00" {
		t.Errorf("got %+v, want color=#00ff00", tags)
	}
}

// --- DeleteTagSoft / UndoDeleteTag ---

func TestDeleteTagSoft_HidesFromReadTags(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("driving", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags after soft-delete, want 0", len(tags))
	}
}

func TestUndoDeleteTag_RestoresTag(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("driving", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}
	if err := w.UndoDeleteTag(id); err != nil {
		t.Fatalf("UndoDeleteTag: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 {
		t.Errorf("got %d tags after undo, want 1", len(tags))
	}
}

// --- PurgeDeletedTags ---

func TestPurgeDeletedTags_RemovesOld(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("old-tag", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	// Purge everything deleted before a very large timestamp.
	if err := w.PurgeDeletedTags(9999999999); err != nil {
		t.Fatalf("PurgeDeletedTags: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("tag still exists after purge")
	}
}

func TestPurgeDeletedTags_SparesFutureDeletions(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("keep-me", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	// Purge with before=0 — nothing should be deleted.
	if err := w.PurgeDeletedTags(0); err != nil {
		t.Fatalf("PurgeDeletedTags: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("tag was incorrectly purged")
	}
}

// --- AddTrackTag / RemoveTrackTag ---

func TestAddTrackTag_Basic(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "hash1", "/music/a.flac")

	id, err := w.CreateTag("chill", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.AddTrackTag("hash1", id); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	tags, err := d.ReadTagsForTrack("hash1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "chill" {
		t.Errorf("got %+v, want [chill]", tags)
	}
}

func TestAddTrackTag_Idempotent(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "hash1", "/music/a.flac")

	id, _ := w.CreateTag("chill", "")
	if err := w.AddTrackTag("hash1", id); err != nil {
		t.Fatalf("first AddTrackTag: %v", err)
	}
	if err := w.AddTrackTag("hash1", id); err != nil {
		t.Fatalf("second AddTrackTag (idempotent): %v", err)
	}

	tags, _ := d.ReadTagsForTrack("hash1")
	if len(tags) != 1 {
		t.Errorf("got %d tags, want 1 (idempotent add doubled)", len(tags))
	}
}

func TestRemoveTrackTag_Basic(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "hash1", "/music/a.flac")

	id, _ := w.CreateTag("chill", "")
	_ = w.AddTrackTag("hash1", id)

	if err := w.RemoveTrackTag("hash1", id); err != nil {
		t.Fatalf("RemoveTrackTag: %v", err)
	}

	tags, _ := d.ReadTagsForTrack("hash1")
	if len(tags) != 0 {
		t.Errorf("got %d tags after remove, want 0", len(tags))
	}
}

func TestRemoveTrackTag_NoOp(t *testing.T) {
	_, w := openTagDB(t)

	// Should not error even when the row doesn't exist.
	if err := w.RemoveTrackTag("nonexistent", 999); err != nil {
		t.Fatalf("RemoveTrackTag no-op: %v", err)
	}
}

// --- AddTrackTagBulk / RemoveTrackTagBulk ---

func TestAddTrackTagBulk(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")
	seedTrack(t, d, "h3", "/c.flac")

	id, _ := w.CreateTag("workout", "")
	if err := w.AddTrackTagBulk([]string{"h1", "h2", "h3"}, id); err != nil {
		t.Fatalf("AddTrackTagBulk: %v", err)
	}

	hashes, err := d.ReadTrackHashesByTag(id)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag: %v", err)
	}
	if len(hashes) != 3 {
		t.Errorf("got %d hashes, want 3", len(hashes))
	}
}

func TestAddTrackTagBulk_EmptyNoop(t *testing.T) {
	_, w := openTagDB(t)

	if err := w.AddTrackTagBulk(nil, 1); err != nil {
		t.Fatalf("AddTrackTagBulk empty: %v", err)
	}
}

func TestAddTrackTagBulk_Idempotent(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("focus", "")
	_ = w.AddTrackTag("h1", id)

	// Bulk add same hash again — should not error.
	if err := w.AddTrackTagBulk([]string{"h1"}, id); err != nil {
		t.Fatalf("AddTrackTagBulk idempotent: %v", err)
	}

	hashes, _ := d.ReadTrackHashesByTag(id)
	if len(hashes) != 1 {
		t.Errorf("got %d hashes, want 1", len(hashes))
	}
}

func TestRemoveTrackTagBulk(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("chill", "")
	_ = w.AddTrackTagBulk([]string{"h1", "h2"}, id)

	if err := w.RemoveTrackTagBulk([]string{"h1", "h2"}, id); err != nil {
		t.Fatalf("RemoveTrackTagBulk: %v", err)
	}

	hashes, _ := d.ReadTrackHashesByTag(id)
	if len(hashes) != 0 {
		t.Errorf("got %d hashes after bulk remove, want 0", len(hashes))
	}
}

func TestRemoveTrackTagBulk_EmptyNoop(t *testing.T) {
	_, w := openTagDB(t)

	if err := w.RemoveTrackTagBulk(nil, 1); err != nil {
		t.Fatalf("RemoveTrackTagBulk empty: %v", err)
	}
}

// --- DeleteTagIfEmpty ---

func TestDeleteTagIfEmpty_DeletesWhenNoTracks(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("empty-tag", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	deleted, err := w.DeleteTagIfEmpty(id)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true for tag with no tracks")
	}

	tags, _ := d.ReadTags()
	if len(tags) != 0 {
		t.Errorf("got %d tags, want 0 after auto-delete", len(tags))
	}
}

func TestDeleteTagIfEmpty_KeepsWhenTracksExist(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("has-tracks", "")
	_ = w.AddTrackTag("h1", id)

	deleted, err := w.DeleteTagIfEmpty(id)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for tag with tracks")
	}

	tags, _ := d.ReadTags()
	if len(tags) != 1 {
		t.Errorf("got %d tags, want 1", len(tags))
	}
}

func TestDeleteTagIfEmpty_DeletesAfterLastTrackRemoved(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("single", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.RemoveTrackTag("h1", id)

	deleted, err := w.DeleteTagIfEmpty(id)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true after last track removed")
	}

	tags, _ := d.ReadTags()
	if len(tags) != 0 {
		t.Errorf("got %d tags, want 0", len(tags))
	}
}

func TestDeleteTagIfEmpty_SkipsAlreadyDeleted(t *testing.T) {
	_, w := openTagDB(t)

	id, _ := w.CreateTag("already-gone", "")
	_ = w.DeleteTagSoft(id)

	// Should return false — the tag is already soft-deleted, the WHERE clause
	// includes "deleted_at IS NULL".
	deleted, err := w.DeleteTagIfEmpty(id)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for already-soft-deleted tag")
	}
}

// --- MergeTags ---

func TestMergeTags_MovesTrackTags(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	srcID, _ := w.CreateTag("source", "")
	dstID, _ := w.CreateTag("dest", "")
	_ = w.AddTrackTag("h1", srcID)
	_ = w.AddTrackTag("h2", srcID)

	if err := w.MergeTags(srcID, dstID); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	hashes, err := d.ReadTrackHashesByTag(dstID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag dest: %v", err)
	}
	if len(hashes) != 2 {
		t.Errorf("dest has %d hashes after merge, want 2", len(hashes))
	}

	srcHashes, _ := d.ReadTrackHashesByTag(srcID)
	if len(srcHashes) != 0 {
		t.Errorf("source has %d hashes after merge, want 0", len(srcHashes))
	}
}

func TestMergeTags_SoftDeletesSource(t *testing.T) {
	d, w := openTagDB(t)

	srcID, _ := w.CreateTag("old", "")
	dstID, _ := w.CreateTag("new", "")

	if err := w.MergeTags(srcID, dstID); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	tags, _ := d.ReadTags()
	for _, tg := range tags {
		if tg.ID == srcID {
			t.Errorf("source tag still visible after merge")
		}
	}
}

func TestMergeTags_OverlapIdempotent(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "shared", "/shared.flac")

	srcID, _ := w.CreateTag("src", "")
	dstID, _ := w.CreateTag("dst", "")
	_ = w.AddTrackTag("shared", srcID)
	_ = w.AddTrackTag("shared", dstID) // already in dest

	if err := w.MergeTags(srcID, dstID); err != nil {
		t.Fatalf("MergeTags overlap: %v", err)
	}

	hashes, _ := d.ReadTrackHashesByTag(dstID)
	if len(hashes) != 1 {
		t.Errorf("got %d hashes after overlap merge, want 1", len(hashes))
	}
}

// --- ReadTags ordering ---

func TestReadTags_OrderedByName(t *testing.T) {
	d, w := openTagDB(t)

	for _, name := range []string{"Zebra", "apple", "Mango"} {
		if _, err := w.CreateTag(name, ""); err != nil {
			t.Fatalf("CreateTag %q: %v", name, err)
		}
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("got %d tags, want 3", len(tags))
	}
	// COLLATE NOCASE ordering: apple < Mango < Zebra
	want := []string{"apple", "Mango", "Zebra"}
	for i, tg := range tags {
		if tg.Name != want[i] {
			t.Errorf("tags[%d].Name = %q, want %q", i, tg.Name, want[i])
		}
	}
}

// --- ReadTagCounts ---

func TestReadTagCounts(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("chill", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.AddTrackTag("h2", id)

	counts, err := d.ReadTagCounts()
	if err != nil {
		t.Fatalf("ReadTagCounts: %v", err)
	}
	if counts[id] != 2 {
		t.Errorf("count[%d] = %d, want 2", id, counts[id])
	}
}

func TestReadTagCounts_Empty(t *testing.T) {
	d, _ := openTagDB(t)

	counts, err := d.ReadTagCounts()
	if err != nil {
		t.Fatalf("ReadTagCounts empty: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("got %d entries, want 0", len(counts))
	}
}

// --- ReadTracksByTag ---

func TestReadTracksByTag(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("focus", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.AddTrackTag("h2", id)

	rows, err := d.ReadTracksByTag(id)
	if err != nil {
		t.Fatalf("ReadTracksByTag: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

func TestReadTracksByTag_SoftDeletedTagExcluded(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("gone", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.DeleteTagSoft(id)

	rows, err := d.ReadTracksByTag(id)
	if err != nil {
		t.Fatalf("ReadTracksByTag: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows for soft-deleted tag, want 0", len(rows))
	}
}

// --- ReadTagsForTrack excludes soft-deleted tags ---

func TestReadTagsForTrack_ExcludesSoftDeleted(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("temp", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.DeleteTagSoft(id)

	tags, err := d.ReadTagsForTrack("h1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags for soft-deleted tag, want 0", len(tags))
	}
}
