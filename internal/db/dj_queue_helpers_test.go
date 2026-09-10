package db_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djTrack seeds a track and returns it, so a queue test can enqueue it without
// tripping the queue -> tracks foreign key. It is idempotent: a test that
// prepares the same selection twice (once refused, once accepted) names the
// same hash twice.
func djTrack(t *testing.T, d *db.DB, hash string) library.Track {
	t.Helper()
	path := "/music/" + hash + ".mp3"
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT OR IGNORE INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		hash, path, now, now,
	); err != nil {
		t.Fatalf("djTrack %q: %v", hash, err)
	}
	return library.Track{Hash: hash, Path: path}
}

// djEntry builds a planner selection carrying one discovery reason, so a test
// that loses the reasons on a round trip fails visibly.
func djEntry(t *testing.T, d *db.DB, hash string) db.DJQueueEntry {
	t.Helper()
	return db.DJQueueEntry{
		Track: djTrack(t, d, hash),
		Reasons: radio.Reasons{
			radio.DiscoveryReason(radio.DiscoveryNever),
			{Code: radio.ReasonIncludeGenre, Value: "Shoegaze"},
		},
	}
}

// djEntries builds one selection per hash.
func djEntries(t *testing.T, d *db.DB, hashes ...string) []db.DJQueueEntry {
	t.Helper()
	entries := make([]db.DJQueueEntry, 0, len(hashes))
	for _, hash := range hashes {
		entries = append(entries, djEntry(t, d, hash))
	}
	return entries
}

// prepareDJBatch prepares an append batch and fails the test on error.
func prepareDJBatch(t *testing.T, w *db.Writer, entries []db.DJQueueEntry) db.DJPreparation {
	t.Helper()
	prep, err := w.PrepareDJBatch(entries)
	if err != nil {
		t.Fatalf("PrepareDJBatch: %v", err)
	}
	return prep
}

// acceptDJBatch accepts a preparation and fails the test on error.
func acceptDJBatch(t *testing.T, w *db.Writer, prep db.DJPreparation, commit db.DJCommit) db.DJAcceptance {
	t.Helper()
	accepted, err := w.AcceptDJBatch(prep, commit)
	if err != nil {
		t.Fatalf("AcceptDJBatch: %v", err)
	}
	return accepted
}

// readQueue reads the visible queue and fails the test on error.
func readQueue(t *testing.T, d *db.DB) []db.QueueRow {
	t.Helper()
	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	return rows
}

// queueHashes returns the visible queue's track hashes in order.
func queueHashes(t *testing.T, d *db.DB) []string {
	t.Helper()
	rows := readQueue(t, d)
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		hashes = append(hashes, row.Hash)
	}
	return hashes
}

// queueRowByID returns the visible queue row with the given id.
func queueRowByID(t *testing.T, d *db.DB, id int64) (db.QueueRow, bool) {
	t.Helper()
	for _, row := range readQueue(t, d) {
		if row.ID == id {
			return row, true
		}
	}
	return db.QueueRow{}, false
}

// queueState is the raw stored state of one queue row, including the columns
// ReadQueue hides.
type queueState struct {
	deletedAt   sql.NullInt64
	djOwned     int
	djReasons   string
	pendingKind string
}

// pending reports whether the row still belongs to an unaccepted preparation.
func (s queueState) pending() bool {
	return s.deletedAt.Valid && s.deletedAt.Int64 == pendingMarker
}

// active reports whether ReadQueue would return the row.
func (s queueState) active() bool { return !s.deletedAt.Valid }

// tombstoned reports whether the row is an ordinary soft-deleted row that the
// purge will eventually reclaim.
func (s queueState) tombstoned() bool { return s.deletedAt.Valid && !s.pending() }

// pendingMarker mirrors the unexported pendingQueueDeletedAt sentinel. Tests
// live in package db_test, so the value is restated here; readQueueState
// asserts nothing else in the schema uses it.
const pendingMarker int64 = 1<<63 - 1

// readQueueState reads the stored provenance and lifecycle columns of one row.
func readQueueState(t *testing.T, d *db.DB, id int64) queueState {
	t.Helper()
	var state queueState
	if err := d.QueryRow(
		`SELECT deleted_at, dj_owned, dj_reasons, pending_kind FROM queue WHERE id=?`, id,
	).Scan(&state.deletedAt, &state.djOwned, &state.djReasons, &state.pendingKind); err != nil {
		t.Fatalf("read queue state for id %d: %v", id, err)
	}
	return state
}

// queueRowExists reports whether the row is still physically present.
func queueRowExists(t *testing.T, d *db.DB, id int64) bool {
	t.Helper()
	return countRows(t, d, `SELECT COUNT(*) FROM queue WHERE id=?`, id) > 0
}

// assertPending fails unless every id names a hidden, DJ-owned pending row.
func assertPending(t *testing.T, d *db.DB, ids []int64) {
	t.Helper()
	for _, id := range ids {
		state := readQueueState(t, d, id)
		if !state.pending() {
			t.Errorf("queue row %d: deleted_at = %v, want the pending marker", id, state.deletedAt)
		}
		if state.pendingKind == "" {
			t.Errorf("queue row %d: pending_kind is empty, want a preparation kind", id)
		}
	}
}

// assertVisible fails unless every id names an active row.
func assertVisible(t *testing.T, d *db.DB, ids []int64) {
	t.Helper()
	for _, id := range ids {
		state := readQueueState(t, d, id)
		if !state.active() {
			t.Errorf("queue row %d: deleted_at = %v, want NULL", id, state.deletedAt)
		}
		if state.pendingKind != "" {
			t.Errorf("queue row %d: pending_kind = %q after activation, want empty", id, state.pendingKind)
		}
	}
}

// assertIDs compares two id slices in order.
func assertIDs(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// assertHashes compares two hash slices in order.
func assertHashes(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}
