package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// TestAcceptDJReplacement_PreservesManualRowsAndReportsMapping is the core
// refresh case: applying new settings retires the DJ's own upcoming entries and
// leaves every manual entry with its original row id and relative order.
func TestAcceptDJReplacement_PreservesManualRowsAndReportsMapping(t *testing.T) {
	d, w := openTagDB(t)

	manualFirst, err := w.AppendTrack(djTrack(t, d, "manual1"), "")
	if err != nil {
		t.Fatalf("AppendTrack manual1: %v", err)
	}
	old := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	acceptDJBatch(t, w, old, db.DJCommit{})
	manualLast, err := w.AppendTrack(djTrack(t, d, "manual2"), "Chill Mix")
	if err != nil {
		t.Fatalf("AppendTrack manual2: %v", err)
	}

	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj3", "dj4"), old.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	if refresh.Kind != db.DJPrepareReplace {
		t.Errorf("refresh.Kind = %q, want %q", refresh.Kind, db.DJPrepareReplace)
	}
	assertHashes(t, "visible queue while the refresh is pending",
		queueHashes(t, d), []string{"manual1", "dj1", "dj2", "manual2"})

	carry := radio.Allocation{CarryUnits: -10}
	accepted := acceptDJBatch(t, w, refresh, db.DJCommit{Allocation: &carry})

	assertIDs(t, "accepted.Removed", accepted.Removed, old.IDs)
	assertIDs(t, "accepted.Activated", accepted.Activated, refresh.IDs)
	want := []db.DJReplacement{
		{OldID: old.IDs[0], NewID: refresh.IDs[0]},
		{OldID: old.IDs[1], NewID: refresh.IDs[1]},
	}
	if len(accepted.Replacements) != len(want) {
		t.Fatalf("accepted.Replacements = %v, want %v", accepted.Replacements, want)
	}
	for i, pair := range accepted.Replacements {
		if pair != want[i] {
			t.Errorf("accepted.Replacements[%d] = %+v, want %+v", i, pair, want[i])
		}
	}

	assertHashes(t, "visible queue after the refresh",
		queueHashes(t, d), []string{"manual1", "manual2", "dj3", "dj4"})

	rows := readQueue(t, d)
	if rows[0].ID != manualFirst || rows[1].ID != manualLast {
		t.Errorf("manual row ids = %d, %d after the refresh, want %d, %d",
			rows[0].ID, rows[1].ID, manualFirst, manualLast)
	}
	if rows[0].DJOwned || rows[1].DJOwned {
		t.Error("a manual row reports DJ ownership after a refresh")
	}
	if rows[1].SourcePlaylist != "Chill Mix" {
		t.Errorf("manual source playlist = %q, want %q", rows[1].SourcePlaylist, "Chill Mix")
	}
	if !rows[2].DJOwned || !rows[3].DJOwned {
		t.Error("a refreshed row does not report DJ ownership")
	}
	if got := readDJSession(t, d).Allocation.CarryUnits; got != -10 {
		t.Errorf("carry units = %d after the refresh, want -10", got)
	}
}

// TestAcceptDJReplacement_ShorterBatchLeavesUnpairedRemovals verifies the
// mapping contract when the planner comes up short: every retired row is
// reported, and the one with no successor pairs with 0 rather than being
// dropped from the mapping.
func TestAcceptDJReplacement_ShorterBatchLeavesUnpairedRemovals(t *testing.T) {
	d, w := openTagDB(t)

	old := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2", "dj3"))
	acceptDJBatch(t, w, old, db.DJCommit{})

	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj4"), old.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	accepted := acceptDJBatch(t, w, refresh, db.DJCommit{})

	if len(accepted.Replacements) != 3 {
		t.Fatalf("accepted.Replacements = %v, want one entry per retired row", accepted.Replacements)
	}
	if accepted.Replacements[0].NewID != refresh.IDs[0] {
		t.Errorf("first replacement = %+v, want NewID %d", accepted.Replacements[0], refresh.IDs[0])
	}
	for _, pair := range accepted.Replacements[1:] {
		if pair.NewID != 0 {
			t.Errorf("unpaired replacement = %+v, want NewID 0", pair)
		}
	}
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj4"})
}

// TestAcceptDJReplacement_SkipsRowsTheUserAlreadyRemoved verifies that a track
// the user dequeued while the refresh was being planned does not cost them the
// refresh: the vanished row is skipped and everything else still lands.
func TestAcceptDJReplacement_SkipsRowsTheUserAlreadyRemoved(t *testing.T) {
	d, w := openTagDB(t)

	old := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	acceptDJBatch(t, w, old, db.DJCommit{})

	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj3"), old.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	// The user dequeues one of the targets after the plan was prepared.
	if err := w.DequeueTrackSoft(old.IDs[0]); err != nil {
		t.Fatalf("DequeueTrackSoft: %v", err)
	}

	accepted := acceptDJBatch(t, w, refresh, db.DJCommit{})
	assertIDs(t, "accepted.Removed", accepted.Removed, []int64{old.IDs[1]})
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj3"})
}

// TestAcceptDJReplacement_DuplicateTargetsRetireOnce guards the pairing against
// a caller that lists the same row twice, which would otherwise emit a bogus
// second mapping entry for a row that no longer exists.
func TestAcceptDJReplacement_DuplicateTargetsRetireOnce(t *testing.T) {
	d, w := openTagDB(t)

	old := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, old, db.DJCommit{})

	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj2"), []int64{old.IDs[0], old.IDs[0]})
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	accepted := acceptDJBatch(t, w, refresh, db.DJCommit{})

	assertIDs(t, "accepted.Removed", accepted.Removed, []int64{old.IDs[0]})
	if len(accepted.Replacements) != 1 {
		t.Errorf("accepted.Replacements = %v, want one entry", accepted.Replacements)
	}
}

// TestAcceptDJReplacement_RetiredRowsStayUndoable verifies the retired rows are
// soft-deleted rather than erased, so the ordinary undo window still covers a
// refresh the user wants back.
func TestAcceptDJReplacement_RetiredRowsStayUndoable(t *testing.T) {
	d, w := openTagDB(t)

	old := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, old, db.DJCommit{})

	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj2"), old.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	acceptDJBatch(t, w, refresh, db.DJCommit{})

	state := readQueueState(t, d, old.IDs[0])
	if !state.tombstoned() {
		t.Fatalf("retired row deleted_at = %v, want an ordinary tombstone", state.deletedAt)
	}
	if err := w.UndoDequeue(old.IDs[0]); err != nil {
		t.Fatalf("UndoDequeue: %v", err)
	}

	restored, ok := queueRowByID(t, d, old.IDs[0])
	if !ok {
		t.Fatal("retired row did not come back after UndoDequeue")
	}
	if !restored.DJOwned {
		t.Error("restored row lost its DJ ownership")
	}
	if !restored.DJReasons.Has(radio.ReasonDiscoveryNever) {
		t.Errorf("restored row reasons = %v, want the original selection reasons", restored.DJReasons)
	}
}
