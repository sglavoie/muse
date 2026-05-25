package db_test

import (
	"strings"
	"testing"
)

// TestMergeTags_SelfMergeRejected verifies that MergeTags refuses to merge a
// tag into itself. Before the fix the method would INSERT OR IGNORE (no-op),
// DELETE every track_tags row for that tag, and soft-delete the tag, silently
// wiping all assignments. The guard now returns an error and the tag remains
// intact.
func TestMergeTags_SelfMergeRejected(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	tagID, err := w.CreateTag("keep-me", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.AddTrackTag("h1", tagID); err != nil {
		t.Fatalf("AddTrackTag h1: %v", err)
	}
	if err := w.AddTrackTag("h2", tagID); err != nil {
		t.Fatalf("AddTrackTag h2: %v", err)
	}

	err = w.MergeTags(tagID, tagID)
	if err == nil {
		t.Fatal("MergeTags(x, x) returned nil error; want self-merge guard error")
	}
	if !strings.Contains(err.Error(), "source and destination") {
		t.Errorf("error = %q, want mention of source and destination", err.Error())
	}

	hashes, err := d.ReadTrackHashesByTag(tagID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag: %v", err)
	}
	if len(hashes) != 2 {
		t.Errorf("track_tags row count after rejected self-merge = %d, want 2 (rows must not be deleted)", len(hashes))
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	found := false
	for _, tg := range tags {
		if tg.ID == tagID {
			found = true
			break
		}
	}
	if !found {
		t.Error("tag was soft-deleted by self-merge; expected guard to short-circuit before any writes")
	}
}

// TestMergeTags_DistinctIDsStillWorks is a regression guard ensuring the
// self-merge guard does not break the normal merge path.
func TestMergeTags_DistinctIDsStillWorks(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	srcID, err := w.CreateTag("src", "")
	if err != nil {
		t.Fatalf("CreateTag src: %v", err)
	}
	dstID, err := w.CreateTag("dst", "")
	if err != nil {
		t.Fatalf("CreateTag dst: %v", err)
	}
	if err := w.AddTrackTag("h1", srcID); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	if err := w.MergeTags(srcID, dstID); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	dstHashes, err := d.ReadTrackHashesByTag(dstID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag dst: %v", err)
	}
	if len(dstHashes) != 1 || dstHashes[0] != "h1" {
		t.Errorf("dst tag hashes = %v, want [h1]", dstHashes)
	}
}

// TestMergeTags_SoftDeletedDestinationRejected verifies that MergeTags refuses
// to merge into a destination tag whose deleted_at is set. Before the fix the
// re-pointed track_tags rows would attach to a soft-deleted tag (invisible to
// ReadTagsForTrack, which filters tags.deleted_at IS NULL) while the source
// was also soft-deleted in the same transaction, silently destroying the
// user's tag assignments.
func TestMergeTags_SoftDeletedDestinationRejected(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	srcID, err := w.CreateTag("src", "")
	if err != nil {
		t.Fatalf("CreateTag src: %v", err)
	}
	dstID, err := w.CreateTag("dst", "")
	if err != nil {
		t.Fatalf("CreateTag dst: %v", err)
	}
	if err := w.AddTrackTag("h1", srcID); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	if err := w.DeleteTagSoft(dstID); err != nil {
		t.Fatalf("DeleteTagSoft dst: %v", err)
	}

	err = w.MergeTags(srcID, dstID)
	if err == nil {
		t.Fatal("MergeTags into soft-deleted destination returned nil error; want guard error")
	}
	if !strings.Contains(err.Error(), "soft-deleted") {
		t.Errorf("error = %q, want mention of soft-deleted destination", err.Error())
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	foundSrc := false
	for _, tg := range tags {
		if tg.ID == srcID {
			foundSrc = true
			break
		}
	}
	if !foundSrc {
		t.Error("source tag was soft-deleted by failed merge; expected rollback")
	}

	srcHashes, err := d.ReadTrackHashesByTag(srcID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag src: %v", err)
	}
	if len(srcHashes) != 1 || srcHashes[0] != "h1" {
		t.Errorf("source track_tags after rejected merge = %v, want [h1]", srcHashes)
	}

	got, err := d.ReadTagsForTrack("h1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(got) != 1 || got[0].ID != srcID {
		t.Errorf("ReadTagsForTrack(h1) = %v, want one row with ID %d (source still active)", got, srcID)
	}
}
