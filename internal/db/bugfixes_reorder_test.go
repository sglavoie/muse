package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestReorderPlaylistTrack_StaleIDIsNoOp verifies that reordering a non-existent
// or already-soft-deleted playlist_tracks row is a silent no-op rather than
// surfacing sql.ErrNoRows. Before the fix the UI would receive a noisy error
// any time it acted on a stale ID (e.g. after a concurrent remove or after the
// undo window purged the row).
func TestReorderPlaylistTrack_StaleIDIsNoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "stale-pt", "/s.flac")
	plID := seedPlaylist(t, d, "Stale PT")
	liveID := seedPlaylistTrack(t, d, plID, "stale-pt", 1000)

	// Case 1: a totally bogus ID.
	const ghostID int64 = 999999
	if err := w.ReorderPlaylistTrack(plID, ghostID, +1); err != nil {
		t.Errorf("ReorderPlaylistTrack(ghost) = %v, want nil (stale IDs must be no-ops)", err)
	}

	// Case 2: an ID that exists but is soft-deleted.
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at = ? WHERE id = ?`, time.Now().Unix(), liveID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	if err := w.ReorderPlaylistTrack(plID, liveID, -1); err != nil {
		t.Errorf("ReorderPlaylistTrack(soft-deleted) = %v, want nil (stale IDs must be no-ops)", err)
	}

	// Case 3: an ID that exists in a different playlist.
	otherPL := seedPlaylist(t, d, "Other PT")
	if err := w.ReorderPlaylistTrack(otherPL, liveID, -1); err != nil {
		t.Errorf("ReorderPlaylistTrack(wrong-playlist) = %v, want nil (cross-playlist IDs must be no-ops)", err)
	}
}

// TestReorderQueueTrack_StaleIDIsNoOp is the queue equivalent of the playlist
// case above. The UI may invoke reorder with an ID that was just dequeued by
// another tick of the event loop; such races must not surface sql.ErrNoRows.
func TestReorderQueueTrack_StaleIDIsNoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "stale-q", "/sq.flac")
	liveID := seedQueueEntry(t, d, "stale-q", 1000)

	const ghostID int64 = 999999
	if err := w.ReorderQueueTrack(ghostID, +1); err != nil {
		t.Errorf("ReorderQueueTrack(ghost) = %v, want nil (stale IDs must be no-ops)", err)
	}

	if _, err := d.Exec(`UPDATE queue SET deleted_at = ? WHERE id = ?`, time.Now().Unix(), liveID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	if err := w.ReorderQueueTrack(liveID, -1); err != nil {
		t.Errorf("ReorderQueueTrack(soft-deleted) = %v, want nil (stale IDs must be no-ops)", err)
	}
}
