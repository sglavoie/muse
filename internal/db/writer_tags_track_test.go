package db_test

import "testing"

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

	if err := w.RemoveTrackTag("nonexistent", 999); err != nil {
		t.Fatalf("RemoveTrackTag no-op: %v", err)
	}
}

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

func TestRemoveAllTrackTags(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id1, _ := w.CreateTag("chill", "")
	id2, _ := w.CreateTag("focus", "")
	_ = w.AddTrackTag("h1", id1)
	_ = w.AddTrackTag("h1", id2)

	// Verify both tags are assigned.
	tags, _ := d.ReadTagsForTrack("h1")
	if len(tags) != 2 {
		t.Fatalf("setup: got %d tags, want 2", len(tags))
	}

	if err := w.RemoveAllTrackTags("h1"); err != nil {
		t.Fatalf("RemoveAllTrackTags: %v", err)
	}

	tags, _ = d.ReadTagsForTrack("h1")
	if len(tags) != 0 {
		t.Errorf("got %d tags after RemoveAllTrackTags, want 0", len(tags))
	}
}

func TestRemoveAllTrackTags_NoOp(t *testing.T) {
	_, w := openTagDB(t)

	// Should not error on a non-existent track.
	if err := w.RemoveAllTrackTags("nonexistent"); err != nil {
		t.Fatalf("RemoveAllTrackTags no-op: %v", err)
	}
}

func TestRemoveAllTrackTags_DoesNotAffectOtherTracks(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("shared", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.AddTrackTag("h2", id)

	if err := w.RemoveAllTrackTags("h1"); err != nil {
		t.Fatalf("RemoveAllTrackTags: %v", err)
	}

	// h2 should still have the tag.
	tags, _ := d.ReadTagsForTrack("h2")
	if len(tags) != 1 {
		t.Errorf("h2: got %d tags, want 1 (unaffected)", len(tags))
	}
}
