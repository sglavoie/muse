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

// TestSetPlaylistOrder_PartialIDsRelocateUntouchedActiveRows verifies that
// SetPlaylistOrder does not violate the UNIQUE (playlist_id, position) index
// when ids is a strict subset of the playlist's active rows. Phase 3 of
// setGappedOrderTx writes positions gap..N*gap to the supplied ids, but those
// positions may already be held by untouched active rows. Before the fix the
// call returned a SQLITE_CONSTRAINT error; the fix relocates the displaced
// active rows to safe positions above the temp zone before phase 3 runs.
func TestSetPlaylistOrder_PartialIDsRelocateUntouchedActiveRows(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "part1", "/part1")
	seedTrack(t, d, "part2", "/part2")
	seedTrack(t, d, "part3", "/part3")
	seedTrack(t, d, "part4", "/part4")
	plID := seedPlaylist(t, d, "Partial Reorder")
	id1 := seedPlaylistTrack(t, d, plID, "part1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "part2", 2000)
	id3 := seedPlaylistTrack(t, d, plID, "part3", 3000)
	id4 := seedPlaylistTrack(t, d, plID, "part4", 4000)

	if err := w.SetPlaylistOrder(plID, []int64{id4, id3}); err != nil {
		t.Fatalf("SetPlaylistOrder partial: %v", err)
	}

	// Verify the targeted ids landed at 1000, 2000 and that the untouched
	// active rows (id1, id2) were relocated rather than dropped or overwritten.
	pos := map[int64]int64{}
	rows, err := d.Query(`SELECT id, position FROM playlist_tracks WHERE playlist_id=? AND deleted_at IS NULL`, plID)
	if err != nil {
		t.Fatalf("query positions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, p int64
		if err := rows.Scan(&id, &p); err != nil {
			t.Fatalf("scan positions: %v", err)
		}
		pos[id] = p
	}
	if pos[id4] != 1000 {
		t.Errorf("id4 position = %d, want 1000", pos[id4])
	}
	if pos[id3] != 2000 {
		t.Errorf("id3 position = %d, want 2000", pos[id3])
	}
	if _, ok := pos[id1]; !ok {
		t.Errorf("id1 was dropped from playlist (still expected to exist)")
	}
	if _, ok := pos[id2]; !ok {
		t.Errorf("id2 was dropped from playlist (still expected to exist)")
	}
	if pos[id1] == pos[id2] || pos[id1] == pos[id3] || pos[id1] == pos[id4] {
		t.Errorf("id1 position %d collides with another row", pos[id1])
	}
	if pos[id2] == pos[id3] || pos[id2] == pos[id4] {
		t.Errorf("id2 position %d collides with another row", pos[id2])
	}
}

// TestSetQueueOrder_PartialIDsRelocateUntouchedActiveRows is the queue
// equivalent of the playlist test above. The queue_pos_idx unique constraint
// covers position alone (no scope column), so the bug surfaces identically.
func TestSetQueueOrder_PartialIDsRelocateUntouchedActiveRows(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qpart1", "/qpart1")
	seedTrack(t, d, "qpart2", "/qpart2")
	seedTrack(t, d, "qpart3", "/qpart3")
	id1 := seedQueueEntry(t, d, "qpart1", 1000)
	id2 := seedQueueEntry(t, d, "qpart2", 2000)
	id3 := seedQueueEntry(t, d, "qpart3", 3000)

	// Reorder only the latter two; without the fix this would collide with id1
	// still holding position 1000.
	if err := w.SetQueueOrder([]int64{id3, id2}); err != nil {
		t.Fatalf("SetQueueOrder partial: %v", err)
	}

	pos := map[int64]int64{}
	rows, err := d.Query(`SELECT id, position FROM queue WHERE deleted_at IS NULL`)
	if err != nil {
		t.Fatalf("query positions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, p int64
		if err := rows.Scan(&id, &p); err != nil {
			t.Fatalf("scan positions: %v", err)
		}
		pos[id] = p
	}
	if pos[id3] != 1000 {
		t.Errorf("id3 position = %d, want 1000", pos[id3])
	}
	if pos[id2] != 2000 {
		t.Errorf("id2 position = %d, want 2000", pos[id2])
	}
	if pos[id1] == pos[id2] || pos[id1] == pos[id3] {
		t.Errorf("id1 position %d collides with another row", pos[id1])
	}
}
