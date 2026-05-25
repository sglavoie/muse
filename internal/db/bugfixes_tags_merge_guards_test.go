package db_test

import (
	"strings"
	"testing"
)

// TestMergeTags_MissingDestinationRejected verifies that MergeTags refuses to
// merge into a destination ID that does not exist in the tags table. Without
// this guard the re-pointed track_tags rows would reference a nonexistent
// tag_id, surviving only because SQLite's foreign key enforcement combined
// with the unique (track_hash, tag_id) index makes the behaviour observable
// downstream rather than at the INSERT site.
func TestMergeTags_MissingDestinationRejected(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	srcID, err := w.CreateTag("src", "")
	if err != nil {
		t.Fatalf("CreateTag src: %v", err)
	}
	if err := w.AddTrackTag("h1", srcID); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	const ghostDest int64 = 99999
	err = w.MergeTags(srcID, ghostDest)
	if err == nil {
		t.Fatal("MergeTags into nonexistent destination returned nil error; want guard error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want mention of missing destination", err.Error())
	}

	srcHashes, err := d.ReadTrackHashesByTag(srcID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag src: %v", err)
	}
	if len(srcHashes) != 1 {
		t.Errorf("source track_tags after rejected merge = %d, want 1", len(srcHashes))
	}
}

// TestMergeTags_MissingSourceRejected verifies that MergeTags refuses to merge
// from a source ID that does not exist. Without this guard the operation
// silently completed (zero rows re-pointed, no source row to soft-delete) and
// returned nil, masking caller bugs that pass stale or wrong source IDs.
func TestMergeTags_MissingSourceRejected(t *testing.T) {
	_, w := openTagDB(t)

	dstID, err := w.CreateTag("dst-only", "")
	if err != nil {
		t.Fatalf("CreateTag dst: %v", err)
	}

	const ghostSrc int64 = 999999
	err = w.MergeTags(ghostSrc, dstID)
	if err == nil {
		t.Fatal("MergeTags(missing-src, dst) returned nil; want guard error")
	}
	if !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want mention of missing source tag", err.Error())
	}
}

// TestMergeTags_SoftDeletedSourceRejected verifies the symmetric case where the
// source tag is itself already soft-deleted: the merge must reject rather than
// silently succeed with zero re-pointed rows.
func TestMergeTags_SoftDeletedSourceRejected(t *testing.T) {
	_, w := openTagDB(t)

	srcID, err := w.CreateTag("src-soft", "")
	if err != nil {
		t.Fatalf("CreateTag src: %v", err)
	}
	dstID, err := w.CreateTag("dst-soft", "")
	if err != nil {
		t.Fatalf("CreateTag dst: %v", err)
	}
	if err := w.DeleteTagSoft(srcID); err != nil {
		t.Fatalf("DeleteTagSoft src: %v", err)
	}

	err = w.MergeTags(srcID, dstID)
	if err == nil {
		t.Fatal("MergeTags from soft-deleted source returned nil; want guard error")
	}
	if !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "soft-deleted") {
		t.Errorf("error = %q, want mention of soft-deleted source", err.Error())
	}
}
