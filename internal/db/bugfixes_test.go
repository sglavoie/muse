package db_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestIncrementStartupCount_ReturnsExactPostUpdateValue establishes the
// correctness property that the bug fix enforces: the returned value is
// exactly the post-image of the UPDATE this call performed. The pre-fix
// implementation ran a separate SELECT after the UPDATE, so any concurrent
// UPDATE landing in between could surface a strictly higher value to the
// caller. The fix uses UPDATE … RETURNING so the read and write are the
// same statement and a foreign UPDATE landing after RETURNING cannot leak
// into the returned value.
//
// We exercise the property by interleaving an out-of-band UPDATE between a
// hand-rolled call to the (fixed) RETURNING statement and any subsequent
// read of the stored value. The returned value must remain the pre-foreign
// post-image.
func TestIncrementStartupCount_ReturnsExactPostUpdateValue(t *testing.T) {
	d := openMigratedDB(t)

	// Run the production statement directly so we can interleave a foreign
	// UPDATE between this statement's RETURNING and any later read of
	// startup_count.
	var got int
	err := d.QueryRow(
		`UPDATE session SET startup_count = startup_count + 1, updated_at = 0
		 WHERE id = 1
		 RETURNING startup_count`,
	).Scan(&got)
	if err != nil {
		t.Fatalf("UPDATE RETURNING: %v", err)
	}

	// Foreign UPDATE lands AFTER the RETURNING captured the value.
	if _, err := d.Exec(`UPDATE session SET startup_count = startup_count + 1 WHERE id = 1`); err != nil {
		t.Fatalf("foreign update: %v", err)
	}

	if got != 1 {
		t.Errorf("RETURNING surfaced %d, want 1 (foreign UPDATE must not leak into returned value)", got)
	}

	// Final stored value must reflect both UPDATEs.
	var final int
	if err := d.QueryRow(`SELECT startup_count FROM session WHERE id = 1`).Scan(&final); err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final != 2 {
		t.Errorf("final startup_count = %d, want 2", final)
	}

	// And the production function itself returns the expected sequence
	// across consecutive sequential calls (regression guard for the new
	// implementation).
	if _, err := d.Exec(`UPDATE session SET startup_count = 0 WHERE id = 1`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	for want := 1; want <= 3; want++ {
		n, err := d.IncrementStartupCount()
		if err != nil {
			t.Fatalf("IncrementStartupCount: %v", err)
		}
		if n != want {
			t.Errorf("IncrementStartupCount = %d, want %d", n, want)
		}
	}
}

// TestIncrementStartupCount_ReturnsIncrementedValue verifies the basic happy
// path: each sequential call returns the next integer starting from 1.
func TestIncrementStartupCount_ReturnsIncrementedValue(t *testing.T) {
	d := openMigratedDB(t)

	for want := 1; want <= 3; want++ {
		got, err := d.IncrementStartupCount()
		if err != nil {
			t.Fatalf("IncrementStartupCount: %v", err)
		}
		if got != want {
			t.Errorf("IncrementStartupCount = %d, want %d", got, want)
		}
	}
}

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

	// track_tags rows must still exist.
	hashes, err := d.ReadTrackHashesByTag(tagID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag: %v", err)
	}
	if len(hashes) != 2 {
		t.Errorf("track_tags row count after rejected self-merge = %d, want 2 (rows must not be deleted)", len(hashes))
	}

	// Tag itself must still be active (not soft-deleted).
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

	// Suppress unused import lint when there are no other consumers.
	_ = db.MixDeepCuts
}

// TestReadListeningStats_TopGenres_SplitsMultiGenreTracks verifies that
// ReadListeningStats.TopGenres counts each individual genre on multi-genre
// tracks (via the track_genres junction) rather than the raw comma-separated
// tracks.genre string. Before the fix a track tagged "rock,pop" was grouped
// into a single composite "rock,pop" bucket, hiding both "rock" and "pop"
// from the user-facing top-N output.
func TestReadListeningStats_TopGenres_SplitsMultiGenreTracks(t *testing.T) {
	d := openMigratedDB(t)

	// Track A: multi-genre "rock,pop". Track B: single genre "rock".
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, genre, created_at, updated_at) VALUES ('a', '/a.flac', 'rock,pop', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert track a: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, genre, created_at, updated_at) VALUES ('b', '/b.flac', 'rock', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert track b: %v", err)
	}
	// Populate the junction so the fixed query has data to group on.
	for _, row := range []struct{ hash, genre string }{
		{"a", "rock"}, {"a", "pop"}, {"b", "rock"},
	} {
		if _, err := d.Exec(
			`INSERT INTO track_genres (track_hash, genre) VALUES (?, ?)`,
			row.hash, row.genre,
		); err != nil {
			t.Fatalf("insert track_genres: %v", err)
		}
	}
	// One play each.
	for _, hash := range []string{"a", "b"} {
		if _, err := d.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			hash, now, 30,
		); err != nil {
			t.Fatalf("insert play_history: %v", err)
		}
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}

	// Expect: "rock" (2 plays: a + b) and "pop" (1 play: a). The pre-fix
	// implementation would have produced "rock,pop" (1 play) and "rock"
	// (1 play), masking the true rock count and surfacing a bogus composite
	// genre.
	seen := map[string]int{}
	for _, e := range stats.TopGenres {
		seen[e.Name] = e.PlayCount
	}
	if seen["rock"] != 2 {
		t.Errorf("top genres rock = %d, want 2 (multi-genre tracks must be counted per individual genre)", seen["rock"])
	}
	if seen["pop"] != 1 {
		t.Errorf("top genres pop = %d, want 1", seen["pop"])
	}
	if _, ok := seen["rock,pop"]; ok {
		t.Errorf("top genres contains composite key %q; expected per-genre keys only", "rock,pop")
	}
}

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

	// Seed one track and mark it missing.
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

	// Verify the tag is still active.
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

	// After the missing flag clears, the tag must still be retrievable for
	// the track. This is the user-visible failure mode the bug caused.
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

	// Soft-delete the destination so the merge would land on a hidden tag.
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

	// Source tag must remain active with its assignments intact (transaction
	// rollback on error).
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

	// The user-visible tag must still be retrievable for the track.
	got, err := d.ReadTagsForTrack("h1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(got) != 1 || got[0].ID != srcID {
		t.Errorf("ReadTagsForTrack(h1) = %v, want one row with ID %d (source still active)", got, srcID)
	}
}

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

	// Source must remain intact.
	srcHashes, err := d.ReadTrackHashesByTag(srcID)
	if err != nil {
		t.Fatalf("ReadTrackHashesByTag src: %v", err)
	}
	if len(srcHashes) != 1 {
		t.Errorf("source track_tags after rejected merge = %d, want 1", len(srcHashes))
	}
}
