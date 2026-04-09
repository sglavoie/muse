package db_test

import "testing"

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

	deleted, err := w.DeleteTagIfEmpty(id)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for already-soft-deleted tag")
	}
}

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
	_ = w.AddTrackTag("shared", dstID)

	if err := w.MergeTags(srcID, dstID); err != nil {
		t.Fatalf("MergeTags overlap: %v", err)
	}

	hashes, _ := d.ReadTrackHashesByTag(dstID)
	if len(hashes) != 1 {
		t.Errorf("got %d hashes after overlap merge, want 1", len(hashes))
	}
}
