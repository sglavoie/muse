package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// TestAppendTrack_WritesManualProvenance pins the manual-entry boundary: every
// explicit enqueue is the user's, whatever display label it carries.
func TestAppendTrack_WritesManualProvenance(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.AppendTrack(djTrack(t, d, "m1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	row, ok := queueRowByID(t, d, id)
	if !ok {
		t.Fatal("appended row missing")
	}
	if row.DJOwned {
		t.Error("AppendTrack produced a DJ-owned row")
	}
	if len(row.DJReasons) != 0 {
		t.Errorf("row.DJReasons = %v, want none for a manual entry", row.DJReasons)
	}
}

// TestCopyingADJRow_ProducesAManualEntry covers the copying rule: a pasted copy
// of a DJ selection belongs to the user, so a later refresh must leave it
// alone even though the original is fair game.
func TestCopyingADJRow_ProducesAManualEntry(t *testing.T) {
	d, w := openTagDB(t)

	original := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, original, db.DJCommit{})

	source, ok := queueRowByID(t, d, original.IDs[0])
	if !ok {
		t.Fatal("DJ row missing after accept")
	}
	// A paste re-enqueues the same track through the ordinary append path.
	copyID, err := w.AppendTrack(
		library.Track{Hash: source.Hash, Path: source.Path}, source.SourcePlaylist,
	)
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	copied, ok := queueRowByID(t, d, copyID)
	if !ok {
		t.Fatal("copied row missing")
	}
	if copied.DJOwned {
		t.Error("a pasted copy of a DJ selection reports DJ ownership")
	}
	if len(copied.DJReasons) != 0 {
		t.Errorf("copied row reasons = %v, want none", copied.DJReasons)
	}

	// The refresh retires the original and leaves the copy in place.
	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj2"), original.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	acceptDJBatch(t, w, refresh, db.DJCommit{})
	assertHashes(t, "visible queue after the refresh", queueHashes(t, d), []string{"dj1", "dj2"})
	if rows := readQueue(t, d); rows[0].ID != copyID {
		t.Errorf("surviving row id = %d, want the copy %d", rows[0].ID, copyID)
	}
}

// TestRestoreQueueTrack_KeepsProvenance covers the undo path that cannot
// restore a row in place and has to recreate it: the recreated entry keeps the
// ownership and reasons the original carried.
func TestRestoreQueueTrack_KeepsProvenance(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})
	original, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatal("DJ row missing after accept")
	}
	if err := w.DequeueTrackSoft(original.ID); err != nil {
		t.Fatalf("DequeueTrackSoft: %v", err)
	}

	restoredID, err := w.RestoreQueueTrack(
		library.Track{Hash: original.Hash, Path: original.Path},
		original.SourcePlaylist,
		db.QueueProvenance{DJOwned: original.DJOwned, Reasons: original.DJReasons},
	)
	if err != nil {
		t.Fatalf("RestoreQueueTrack: %v", err)
	}

	restored, ok := queueRowByID(t, d, restoredID)
	if !ok {
		t.Fatal("restored row missing")
	}
	if !restored.DJOwned {
		t.Error("restored row lost its DJ ownership")
	}
	if restored.DJReasons.Summary() != original.DJReasons.Summary() {
		t.Errorf("restored reasons = %q, want %q",
			restored.DJReasons.Summary(), original.DJReasons.Summary())
	}
}

// TestRestoreQueueTrack_KeepsManualProvenance is the other half: restoring a
// manual entry must not accidentally promote it into something the DJ owns.
func TestRestoreQueueTrack_KeepsManualProvenance(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.RestoreQueueTrack(djTrack(t, d, "m1"), "Chill Mix", db.ManualProvenance())
	if err != nil {
		t.Fatalf("RestoreQueueTrack: %v", err)
	}

	row, ok := queueRowByID(t, d, id)
	if !ok {
		t.Fatal("restored row missing")
	}
	if row.DJOwned {
		t.Error("restoring a manual entry produced a DJ-owned row")
	}
	if row.SourcePlaylist != "Chill Mix" {
		t.Errorf("source playlist = %q, want %q", row.SourcePlaylist, "Chill Mix")
	}
}

// TestUndoClearQueue_PreservesProvenance verifies that clearing and restoring
// the whole queue keeps each row on the side it started on.
func TestUndoClearQueue_PreservesProvenance(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})
	manualID, err := w.AppendTrack(djTrack(t, d, "m1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	cleared, err := w.ClearQueueSoft()
	if err != nil {
		t.Fatalf("ClearQueueSoft: %v", err)
	}
	assertHashes(t, "visible queue after clear", queueHashes(t, d), []string{})
	if err := w.UndoClearQueue(cleared); err != nil {
		t.Fatalf("UndoClearQueue: %v", err)
	}

	djRow, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatal("DJ row missing after undo")
	}
	if !djRow.DJOwned || !djRow.DJReasons.Has(radio.ReasonDiscoveryNever) {
		t.Errorf("DJ row after undo: owned=%v reasons=%v, want owned with reasons",
			djRow.DJOwned, djRow.DJReasons)
	}
	manualRow, ok := queueRowByID(t, d, manualID)
	if !ok {
		t.Fatal("manual row missing after undo")
	}
	if manualRow.DJOwned {
		t.Error("manual row became DJ-owned through clear and undo")
	}
}

// TestClearQueueSoft_IgnoresPendingBatches verifies a clear cannot capture an
// unaccepted preparation. Were it to, UndoClearQueue would publish tracks the
// user never accepted.
func TestClearQueueSoft_IgnoresPendingBatches(t *testing.T) {
	d, w := openTagDB(t)

	visible := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, visible, db.DJCommit{})
	pending := prepareDJBatch(t, w, djEntries(t, d, "dj2"))

	cleared, err := w.ClearQueueSoft()
	if err != nil {
		t.Fatalf("ClearQueueSoft: %v", err)
	}
	assertIDs(t, "cleared ids", cleared, visible.IDs)
	assertPending(t, d, pending.IDs)

	if err := w.UndoClearQueue(append(cleared, pending.IDs...)); err != nil {
		t.Fatalf("UndoClearQueue: %v", err)
	}
	assertPending(t, d, pending.IDs)
	assertHashes(t, "visible queue after undo", queueHashes(t, d), []string{"dj1"})

	// The preparation is untouched and still acceptable.
	acceptDJBatch(t, w, pending, db.DJCommit{})
	assertHashes(t, "visible queue after accepting", queueHashes(t, d), []string{"dj1", "dj2"})
}

// TestUndoDequeue_DoesNotPublishAPendingBatch is the same guard on the
// single-row undo path: a stale row id from the UI must not activate a hidden
// batch.
func TestUndoDequeue_DoesNotPublishAPendingBatch(t *testing.T) {
	d, w := openTagDB(t)

	pending := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	if err := w.UndoDequeue(pending.IDs[0]); err != nil {
		t.Fatalf("UndoDequeue: %v", err)
	}
	assertPending(t, d, pending.IDs)
	assertHashes(t, "visible queue", queueHashes(t, d), []string{})
}

// TestDequeueTrackSoft_DoesNotClobberAPendingMarker verifies the reverse: a
// soft delete aimed at a pending row must not turn it into a tombstone, which
// would strand the batch as un-acceptable.
func TestDequeueTrackSoft_DoesNotClobberAPendingMarker(t *testing.T) {
	d, w := openTagDB(t)

	pending := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	if err := w.DequeueTrackSoft(pending.IDs[0]); err != nil {
		t.Fatalf("DequeueTrackSoft: %v", err)
	}
	if err := w.DequeueTracksSoft(pending.IDs); err != nil {
		t.Fatalf("DequeueTracksSoft: %v", err)
	}
	assertPending(t, d, pending.IDs)

	acceptDJBatch(t, w, pending, db.DJCommit{})
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj1", "dj2"})
}

// TestReadQueue_SurvivesUnreadableReasons verifies the degradation policy: a
// selection explanation this build cannot parse costs the explanation, never
// the queue entry or its ownership.
func TestReadQueue_SurvivesUnreadableReasons(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})
	if _, err := d.Exec(`UPDATE queue SET dj_reasons='{not json' WHERE id=?`, prep.IDs[0]); err != nil {
		t.Fatalf("corrupt reasons: %v", err)
	}

	row, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatal("row with unreadable reasons vanished from the queue")
	}
	if !row.DJOwned {
		t.Error("row with unreadable reasons lost its DJ ownership")
	}
	if len(row.DJReasons) != 0 {
		t.Errorf("row.DJReasons = %v, want empty when the stored value is unreadable", row.DJReasons)
	}
}

// TestQueueReasons_RoundTripThroughStorage pins the stored reason shape: code,
// value, and detail all survive, in order.
func TestQueueReasons_RoundTripThroughStorage(t *testing.T) {
	d, w := openTagDB(t)

	reasons := radio.Reasons{
		{Code: radio.ReasonIncludeTag, Value: "7", Detail: "matched an included tag"},
		{Code: radio.ReasonDiscoveryStale, Detail: "last counted play is at least 90 days old"},
	}
	prep, err := w.PrepareDJBatch([]db.DJQueueEntry{{Track: djTrack(t, d, "dj1"), Reasons: reasons}})
	if err != nil {
		t.Fatalf("PrepareDJBatch: %v", err)
	}
	acceptDJBatch(t, w, prep, db.DJCommit{})

	row, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatal("row missing")
	}
	if len(row.DJReasons) != len(reasons) {
		t.Fatalf("row.DJReasons = %v, want %v", row.DJReasons, reasons)
	}
	for i, want := range reasons {
		if row.DJReasons[i] != want {
			t.Errorf("row.DJReasons[%d] = %+v, want %+v", i, row.DJReasons[i], want)
		}
	}
}
