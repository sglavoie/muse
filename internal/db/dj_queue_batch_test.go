package db_test

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// TestPrepareDJBatch_HidesRowsUntilAccepted verifies the whole point of a
// preparation: the rows are durable but invisible, so a crash before acceptance
// cannot surface a batch the user never saw.
func TestPrepareDJBatch_HidesRowsUntilAccepted(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2", "dj3"))

	if prep.Kind != db.DJPrepareAppend {
		t.Errorf("prep.Kind = %q, want %q", prep.Kind, db.DJPrepareAppend)
	}
	if len(prep.IDs) != 3 {
		t.Fatalf("prep.IDs = %v, want 3 ids", prep.IDs)
	}
	if len(prep.Replacing) != 0 {
		t.Errorf("prep.Replacing = %v, want empty for an append", prep.Replacing)
	}
	assertPending(t, d, prep.IDs)
	assertHashes(t, "visible queue while pending", queueHashes(t, d), []string{})
}

// TestAcceptDJBatch_PublishesRowsWithProvenance covers the accept half: the
// rows become visible, keep their DJ ownership, and carry the reasons the
// planner recorded.
func TestAcceptDJBatch_PublishesRowsWithProvenance(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	carry := radio.Allocation{CarryUnits: 30}
	accepted := acceptDJBatch(t, w, prep, db.DJCommit{Allocation: &carry})

	assertIDs(t, "accepted.Activated", accepted.Activated, prep.IDs)
	assertIDs(t, "accepted.Removed", accepted.Removed, []int64{})
	if accepted.Replacements != nil {
		t.Errorf("accepted.Replacements = %v, want nil for an append", accepted.Replacements)
	}
	assertVisible(t, d, prep.IDs)
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj1", "dj2"})

	row, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatalf("queue row %d missing after accept", prep.IDs[0])
	}
	if !row.DJOwned {
		t.Error("accepted row is not DJ-owned")
	}
	if !row.DJReasons.Has(radio.ReasonDiscoveryNever) {
		t.Errorf("row.DJReasons = %v, want the recorded discovery reason", row.DJReasons)
	}
	if !row.DJReasons.Has(radio.ReasonIncludeGenre) {
		t.Errorf("row.DJReasons = %v, want the recorded include reason", row.DJReasons)
	}

	if got := readDJSession(t, d).Allocation.CarryUnits; got != 30 {
		t.Errorf("carry units = %d after accept, want 30", got)
	}
}

// TestAcceptDJBatch_CommitsSettingsWithTheBatch verifies the single-transaction
// requirement for a refresh produced by newly applied settings: the queue rows
// and the settings snapshot land together.
func TestAcceptDJBatch_CommitsSettingsWithTheBatch(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	session := db.DJSession{
		Active:     true,
		Settings:   djTestSettings(rock, live),
		Seed:       djTestSeed(rock),
		Allocation: radio.Allocation{CarryUnits: 20},
	}
	acceptDJBatch(t, w, prep, db.DJCommit{Session: &session})

	assertVisible(t, d, prep.IDs)
	stored := readDJSession(t, d)
	if !stored.Active {
		t.Error("DJ session is not active after accepting a batch that applied settings")
	}
	if stored.Settings.DiscoveryPercent != 60 {
		t.Errorf("discovery percent = %d, want 60", stored.Settings.DiscoveryPercent)
	}
	if stored.Allocation.CarryUnits != 20 {
		t.Errorf("carry units = %d, want 20", stored.Allocation.CarryUnits)
	}
}

// TestAcceptDJBatch_InvalidSettingsLeaveTheBatchPending verifies that DJ state
// is validated before the transaction opens, so a rejected commit costs the
// caller nothing: the batch is still pending and can be retried.
func TestAcceptDJBatch_InvalidSettingsLeaveTheBatchPending(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	// Active with no captured seed: a session that cannot be restored.
	broken := db.DJSession{Active: true, Settings: radio.DefaultSettings()}
	if _, err := w.AcceptDJBatch(prep, db.DJCommit{Session: &broken}); !errors.Is(err, radio.ErrMissingSeed) {
		t.Fatalf("AcceptDJBatch with a seedless active session: err = %v, want ErrMissingSeed", err)
	}

	assertPending(t, d, prep.IDs)
	assertHashes(t, "visible queue", queueHashes(t, d), []string{})

	// Retrying with a usable commit still works.
	acceptDJBatch(t, w, prep, db.DJCommit{})
	assertVisible(t, d, prep.IDs)
}

// TestRejectDJBatch_TombstonesPreparedRows verifies rejection: the rows become
// ordinary soft-deleted rows the purge can reclaim, and never become visible.
func TestRejectDJBatch_TombstonesPreparedRows(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	if err := w.RejectDJBatch(prep); err != nil {
		t.Fatalf("RejectDJBatch: %v", err)
	}

	for _, id := range prep.IDs {
		state := readQueueState(t, d, id)
		if !state.tombstoned() {
			t.Errorf("queue row %d: deleted_at = %v, want an ordinary tombstone", id, state.deletedAt)
		}
		if state.pendingKind != "" {
			t.Errorf("queue row %d: pending_kind = %q after reject, want empty", id, state.pendingKind)
		}
	}
	assertHashes(t, "visible queue after reject", queueHashes(t, d), []string{})

	// A rejected batch is not re-acceptable.
	if _, err := w.AcceptDJBatch(prep, db.DJCommit{}); !errors.Is(err, db.ErrQueueBatchStale) {
		t.Fatalf("AcceptDJBatch after reject: err = %v, want ErrQueueBatchStale", err)
	}
	assertHashes(t, "visible queue after a stale accept", queueHashes(t, d), []string{})
}

// TestRejectDJBatch_AfterRecoveryIsNotAnError verifies that rejecting a batch
// restart recovery already cleaned up is a no-op, so a shutdown race between
// the two paths cannot surface an error to the user.
func TestRejectDJBatch_AfterRecoveryIsNotAnError(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	if err := w.RejectDJBatch(prep); err != nil {
		t.Fatalf("first RejectDJBatch: %v", err)
	}
	if err := w.RejectDJBatch(prep); err != nil {
		t.Fatalf("second RejectDJBatch: %v", err)
	}
}

// TestAcceptDJBatch_StaleIDRollsBackTheWholeTransaction is the failure
// injection for the accept transaction: a stale id in the middle of the batch
// must undo the rows already published ahead of it.
func TestAcceptDJBatch_StaleIDRollsBackTheWholeTransaction(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	carry := radio.Allocation{CarryUnits: 40}
	corrupted := db.DJPreparation{
		Kind: prep.Kind,
		// A row id that never existed, injected after two valid ones.
		IDs: append(append([]int64{}, prep.IDs...), 999_999),
	}

	if _, err := w.AcceptDJBatch(corrupted, db.DJCommit{Allocation: &carry}); !errors.Is(err, db.ErrQueueBatchStale) {
		t.Fatalf("AcceptDJBatch with a stale id: err = %v, want ErrQueueBatchStale", err)
	}

	assertPending(t, d, prep.IDs)
	assertHashes(t, "visible queue after the failed accept", queueHashes(t, d), []string{})
	if got := readDJSession(t, d).Allocation.CarryUnits; got != 0 {
		t.Errorf("carry units = %d after the failed accept, want 0", got)
	}

	// The untouched preparation is still good.
	acceptDJBatch(t, w, prep, db.DJCommit{Allocation: &carry})
	assertHashes(t, "visible queue after retrying", queueHashes(t, d), []string{"dj1", "dj2"})
}

// TestAcceptDJBatch_ProtectedRowRollsBackTheRemovals injects a manual row into
// the replacement set after a DJ-owned one. The DJ row is retired first inside
// the transaction, so a correct rollback is the only thing that brings it back.
func TestAcceptDJBatch_ProtectedRowRollsBackTheRemovals(t *testing.T) {
	d, w := openTagDB(t)

	first := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	acceptDJBatch(t, w, first, db.DJCommit{})
	manualID, err := w.AppendTrack(djTrack(t, d, "manual1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	refresh := prepareDJBatch(t, w, djEntries(t, d, "dj3"))
	corrupted := db.DJPreparation{
		Kind:      db.DJPrepareReplace,
		IDs:       refresh.IDs,
		Replacing: []int64{first.IDs[0], manualID},
	}

	if _, err := w.AcceptDJBatch(corrupted, db.DJCommit{}); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("AcceptDJBatch replacing a manual row: err = %v, want ErrQueueRowProtected", err)
	}

	assertVisible(t, d, first.IDs)
	assertVisible(t, d, []int64{manualID})
	assertPending(t, d, refresh.IDs)
	assertHashes(t, "visible queue after the refused refresh",
		queueHashes(t, d), []string{"dj1", "dj2", "manual1"})
}

// TestPrepareDJReplacement_RefusesProtectedRowBeforeWriting verifies the early
// guard: a caller that names a manual row gets the error before any hidden row
// is written, so no orphaned preparation is left behind.
func TestPrepareDJReplacement_RefusesProtectedRowBeforeWriting(t *testing.T) {
	d, w := openTagDB(t)

	manualID, err := w.AppendTrack(djTrack(t, d, "manual1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	if _, err := w.PrepareDJReplacement(djEntries(t, d, "dj1"), []int64{manualID}); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("PrepareDJReplacement over a manual row: err = %v, want ErrQueueRowProtected", err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM queue WHERE dj_owned = 1`); n != 0 {
		t.Errorf("DJ-owned queue rows = %d after a refused preparation, want 0", n)
	}
}

// TestPrepareDJBatch_RejectsAnAppendThatReplaces guards the caller-error path.
func TestPrepareDJBatch_RejectsAnAppendThatReplaces(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})

	bad := db.DJPreparation{Kind: db.DJPrepareAppend, IDs: nil, Replacing: prep.IDs}
	if _, err := w.AcceptDJBatch(bad, db.DJCommit{}); !errors.Is(err, db.ErrQueuePreparationInvalid) {
		t.Fatalf("AcceptDJBatch of an append that replaces: err = %v, want ErrQueuePreparationInvalid", err)
	}
	assertVisible(t, d, prep.IDs)
}
