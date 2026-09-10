package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// restartWriter simulates a Muse restart over the same database file: the old
// writer is shut down and a fresh one runs its startup recovery.
func restartWriter(t *testing.T, d *db.DB) *db.Writer {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)
	t.Cleanup(func() {
		cancel()
		w.Wait()
	})
	return w
}

// TestRestart_AbandonsPreparedDJBatch verifies restart recovery: a preparation
// that was never accepted is converted into an ordinary tombstone, stays out of
// the queue, and can no longer be published.
func TestRestart_AbandonsPreparedDJBatch(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1", "dj2"))
	assertPending(t, d, prep.IDs)

	restarted := restartWriter(t, d)

	for _, id := range prep.IDs {
		state := readQueueState(t, d, id)
		if !state.tombstoned() {
			t.Errorf("queue row %d: deleted_at = %v after restart, want an ordinary tombstone", id, state.deletedAt)
		}
		if state.pendingKind != "" {
			t.Errorf("queue row %d: pending_kind = %q after restart, want empty", id, state.pendingKind)
		}
	}
	assertHashes(t, "visible queue after restart", queueHashes(t, d), []string{})

	if _, err := restarted.AcceptDJBatch(prep, db.DJCommit{}); !errors.Is(err, db.ErrQueueBatchStale) {
		t.Fatalf("AcceptDJBatch after restart: err = %v, want ErrQueueBatchStale", err)
	}
	assertHashes(t, "visible queue after a post-restart accept", queueHashes(t, d), []string{})
}

// TestRestart_PreservesAcceptedProvenance verifies that ownership and reasons
// survive a restart, so a refresh after relaunching Muse still knows which rows
// it may replace.
func TestRestart_PreservesAcceptedProvenance(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})
	manualID, err := w.AppendTrack(djTrack(t, d, "manual1"), "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	restarted := restartWriter(t, d)

	djRow, ok := queueRowByID(t, d, prep.IDs[0])
	if !ok {
		t.Fatal("DJ row missing after restart")
	}
	if !djRow.DJOwned || len(djRow.DJReasons) == 0 {
		t.Errorf("DJ row after restart: owned=%v reasons=%v, want owned with reasons", djRow.DJOwned, djRow.DJReasons)
	}
	manualRow, ok := queueRowByID(t, d, manualID)
	if !ok {
		t.Fatal("manual row missing after restart")
	}
	if manualRow.DJOwned {
		t.Error("manual row reports DJ ownership after restart")
	}

	// The restored session can still refresh its own rows and only its own.
	if _, err := restarted.PrepareDJReplacement(djEntries(t, d, "dj2"), []int64{manualID}); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("PrepareDJReplacement over the manual row: err = %v, want ErrQueueRowProtected", err)
	}
	refresh, err := restarted.PrepareDJReplacement(djEntries(t, d, "dj2"), prep.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	if _, err := restarted.AcceptDJBatch(refresh, db.DJCommit{}); err != nil {
		t.Fatalf("AcceptDJBatch: %v", err)
	}
	assertHashes(t, "visible queue after the post-restart refresh",
		queueHashes(t, d), []string{"manual1", "dj2"})
}

// TestRestart_LeavesLegacyRadioRowsUnowned pins the migration rule: rows that
// predate provenance — including the pre-DJ radio path's own rows — are manual
// entries the DJ may not replace.
func TestRestart_LeavesLegacyRadioRowsUnowned(t *testing.T) {
	d, w := openTagDB(t)

	ids, err := w.AppendRadioTracksPending([]library.Track{djTrack(t, d, "legacy1")})
	if err != nil {
		t.Fatalf("AppendRadioTracksPending: %v", err)
	}
	if err := w.ActivateRadioTracks(ids); err != nil {
		t.Fatalf("ActivateRadioTracks: %v", err)
	}

	row, ok := queueRowByID(t, d, ids[0])
	if !ok {
		t.Fatal("legacy radio row missing")
	}
	if row.SourcePlaylist != "Radio" {
		t.Errorf("source playlist = %q, want %q", row.SourcePlaylist, "Radio")
	}
	if row.DJOwned {
		t.Error("a legacy Radio row reports DJ ownership; the display label must not imply it")
	}
	if _, err := w.PrepareDJReplacement(djEntries(t, d, "dj1"), ids); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("PrepareDJReplacement over a legacy Radio row: err = %v, want ErrQueueRowProtected", err)
	}
}

// TestSourceLabelCollision_PlaylistNamedRadioStaysManual covers the collision
// the plan calls out: a user playlist literally named "Radio" produces rows the
// DJ must treat as manual, and recovery must not touch them either.
func TestSourceLabelCollision_PlaylistNamedRadioStaysManual(t *testing.T) {
	d, w := openTagDB(t)

	tracks := []library.Track{djTrack(t, d, "user1"), djTrack(t, d, "user2")}
	ids, err := w.AppendTracks(tracks, "Radio")
	if err != nil {
		t.Fatalf("AppendTracks: %v", err)
	}
	single, err := w.AppendTrack(djTrack(t, d, "user3"), "Radio")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}
	ids = append(ids, single)

	for _, id := range ids {
		row, ok := queueRowByID(t, d, id)
		if !ok {
			t.Fatalf("queue row %d missing", id)
		}
		if row.SourcePlaylist != "Radio" {
			t.Errorf("queue row %d source playlist = %q, want %q", id, row.SourcePlaylist, "Radio")
		}
		if row.DJOwned {
			t.Errorf("queue row %d from a playlist named Radio reports DJ ownership", id)
		}
	}

	if _, err := w.PrepareDJReplacement(djEntries(t, d, "dj1"), ids); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("PrepareDJReplacement over playlist rows: err = %v, want ErrQueueRowProtected", err)
	}

	// Restart recovery keys on the pending marker, not on the label, so these
	// visible rows survive untouched.
	restartWriter(t, d)
	assertHashes(t, "visible queue after restart", queueHashes(t, d), []string{"user1", "user2", "user3"})
}

// TestPurge_KeepsPendingBatchesAndReclaimsRejectedOnes verifies the purge
// boundary: an in-flight preparation is never reclaimed, while the tombstones
// left by a rejection or by restart recovery are.
func TestPurge_KeepsPendingBatchesAndReclaimsRejectedOnes(t *testing.T) {
	d, w := openTagDB(t)

	rejected := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	if err := w.RejectDJBatch(rejected); err != nil {
		t.Fatalf("RejectDJBatch: %v", err)
	}
	inFlight := prepareDJBatch(t, w, djEntries(t, d, "dj2"))

	// A cutoff far in the future would sweep everything the timestamp
	// comparison allows; the pending marker must still be excluded.
	future := time.Now().Add(24 * time.Hour).Unix()
	if err := w.PurgeDeletedQueueEntries(future); err != nil {
		t.Fatalf("PurgeDeletedQueueEntries: %v", err)
	}

	if queueRowExists(t, d, rejected.IDs[0]) {
		t.Error("a rejected preparation survived the purge")
	}
	if !queueRowExists(t, d, inFlight.IDs[0]) {
		t.Fatal("an in-flight preparation was purged")
	}
	assertPending(t, d, inFlight.IDs)

	// Still acceptable after the purge ran.
	acceptDJBatch(t, w, inFlight, db.DJCommit{})
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj2"})
}

// TestPurge_ReclaimsRetiredDJRowsWithTheirProvenance verifies that provenance
// columns go away with the row and leave nothing behind.
func TestPurge_ReclaimsRetiredDJRowsWithTheirProvenance(t *testing.T) {
	d, w := openTagDB(t)

	old := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, old, db.DJCommit{})
	refresh, err := w.PrepareDJReplacement(djEntries(t, d, "dj2"), old.IDs)
	if err != nil {
		t.Fatalf("PrepareDJReplacement: %v", err)
	}
	acceptDJBatch(t, w, refresh, db.DJCommit{})

	if err := w.PurgeDeletedQueueEntries(time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("PurgeDeletedQueueEntries: %v", err)
	}

	if queueRowExists(t, d, old.IDs[0]) {
		t.Error("a retired DJ row survived the purge")
	}
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj2"})
	if n := countRows(t, d, `SELECT COUNT(*) FROM queue WHERE dj_reasons != ''`); n != 1 {
		t.Errorf("rows carrying reasons = %d after the purge, want 1", n)
	}
}
