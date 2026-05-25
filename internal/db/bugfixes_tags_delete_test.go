package db_test

import "testing"

// TestDeleteTagIfEmpty_PreservesTagWithMissingTrackAssignments establishes the
// property that DeleteTagIfEmpty must NOT soft-delete a tag while track_tags
// rows still reference it, even if the referenced track is currently flagged
// missing. Before the fix the count filtered missing=0, so a tag whose only
// assignments were on missing tracks reported count=0 and was wiped; the
// orphaned track_tags rows then pointed to a soft-deleted tag and became
// invisible to ReadTagsForTrack (which joins on deleted_at IS NULL),
// silently destroying user data when the file came back from missing.
func TestDeleteTagIfEmpty_PreservesTagWithMissingTrackAssignments(t *testing.T) {
	d, w := openTagDB(t)

	seedTrack(t, d, "h-missing", "/m.flac")
	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = 'h-missing'`); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	tagID, err := w.CreateTag("user-tag", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.AddTrackTag("h-missing", tagID); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	deleted, err := w.DeleteTagIfEmpty(tagID)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if deleted {
		t.Error("DeleteTagIfEmpty soft-deleted a tag that still has track_tags rows (on a missing track); user data would be silently destroyed when the file returns")
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
		t.Error("tag soft-deleted by DeleteTagIfEmpty despite live track_tags row on a missing track")
	}

	if _, err := d.Exec(`UPDATE tracks SET missing = 0 WHERE hash = 'h-missing'`); err != nil {
		t.Fatalf("clear missing: %v", err)
	}
	got, err := d.ReadTagsForTrack("h-missing")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(got) != 1 || got[0].ID != tagID {
		t.Errorf("ReadTagsForTrack(h-missing) = %v, want one row with ID %d", got, tagID)
	}
}

// TestDeleteTagIfEmpty_TrulyEmptyTagStillDeletes is a regression guard: when
// no track_tags rows exist at all the tag must still be soft-deleted.
func TestDeleteTagIfEmpty_TrulyEmptyTagStillDeletes(t *testing.T) {
	_, w := openTagDB(t)
	tagID, err := w.CreateTag("empty", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	deleted, err := w.DeleteTagIfEmpty(tagID)
	if err != nil {
		t.Fatalf("DeleteTagIfEmpty: %v", err)
	}
	if !deleted {
		t.Error("DeleteTagIfEmpty did not delete a tag with zero assignments")
	}
}
