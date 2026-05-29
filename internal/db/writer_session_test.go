package db_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestWriteShuffle_Disable verifies that WriteShuffle(false) writes shuffle=0
// to the session row without modifying shuffle_position values.
func TestWriteShuffle_Disable(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs11", "/f11")
	now := time.Now().Unix()

	// Insert a row with a non-default shuffle_position.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, shuffle_position, added_at) VALUES ('hs11', 1000, 5000, ?)`, now,
	)
	if err != nil {
		t.Fatalf("insert queue row: %v", err)
	}
	id, _ := res.LastInsertId()

	// Set session shuffle=1 first.
	if _, err := d.Exec(`UPDATE session SET shuffle=1, updated_at=? WHERE id=1`, now); err != nil {
		t.Fatalf("set session shuffle: %v", err)
	}

	if err := w.WriteShuffle(false); err != nil {
		t.Fatalf("WriteShuffle(false): %v", err)
	}

	// Session must have shuffle=0.
	var shuffle int
	if err := d.QueryRow(`SELECT shuffle FROM session WHERE id=1`).Scan(&shuffle); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if shuffle != 0 {
		t.Errorf("session.shuffle = %d; want 0", shuffle)
	}

	// shuffle_position must be unchanged.
	var sp int64
	if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
		t.Fatalf("query shuffle_position: %v", err)
	}
	if sp != 5000 {
		t.Errorf("shuffle_position = %d; want 5000 (unchanged)", sp)
	}
}

// TestWriteShuffle_Enable verifies that WriteShuffle(true) assigns non-zero
// shuffle_position values to all active rows inside a single transaction and
// writes shuffle=1 to the session row. Soft-deleted rows are not reordered.
func TestWriteShuffle_Enable(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs10", "/f10")
	now := time.Now().Unix()

	var activeIDs []int64
	for _, pos := range []int{1000, 2000, 3000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs10', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert active row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		activeIDs = append(activeIDs, id)
	}

	// One soft-deleted row — must not be touched.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs10', 4000, ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert soft-deleted row: %v", err)
	}
	deletedID, _ := res.LastInsertId()

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("WriteShuffle(true): %v", err)
	}

	// All active rows must have a positive shuffle_position.
	for _, id := range activeIDs {
		var sp int64
		if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
			t.Fatalf("query shuffle_position for id %d: %v", id, err)
		}
		if sp <= 0 {
			t.Errorf("id %d: shuffle_position = %d; want > 0", id, sp)
		}
	}

	// Soft-deleted row must keep shuffle_position = 0 (default).
	var sp int64
	if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, deletedID).Scan(&sp); err != nil {
		t.Fatalf("query deleted row: %v", err)
	}
	if sp != 0 {
		t.Errorf("soft-deleted row shuffle_position = %d; want 0", sp)
	}

	// Session must have shuffle=1.
	var shuffle int
	if err := d.QueryRow(`SELECT shuffle FROM session WHERE id=1`).Scan(&shuffle); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if shuffle != 1 {
		t.Errorf("session.shuffle = %d; want 1", shuffle)
	}
}

// TestWriteShuffle_TwoEnablesProduceDifferentOrderings verifies that calling
// WriteShuffle(true) twice produces different shuffle_position assignments.
func TestWriteShuffle_TwoEnablesProduceDifferentOrderings(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs12", "/f12")
	now := time.Now().Unix()

	var ids []int64
	for _, pos := range []int{1000, 2000, 3000, 4000, 5000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs12', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	readPositions := func() []int64 {
		var positions []int64
		for _, id := range ids {
			var sp int64
			if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
				t.Fatalf("query shuffle_position for id %d: %v", id, err)
			}
			positions = append(positions, sp)
		}
		return positions
	}

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("first WriteShuffle(true): %v", err)
	}
	first := readPositions()

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("second WriteShuffle(true): %v", err)
	}
	second := readPositions()

	if slices.Equal(first, second) {
		t.Errorf("two consecutive enables produced identical shuffle_position ordering %v; expected different", first)
	}
}

// TestWriteShuffle_ShufflePositionDiffersFromPosition verifies that after
// WriteShuffle(true), the shuffle_position ordering is distinct from the
// original position ordering, confirming that shuffle_position is freshly
// generated and not simply copied from the position column.
func TestWriteShuffle_ShufflePositionDiffersFromPosition(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs13", "/f13")
	now := time.Now().Unix()

	positions := []int{1000, 2000, 3000, 4000, 5000, 6000}
	var ids []int64
	for _, pos := range positions {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs13', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("WriteShuffle(true): %v", err)
	}

	// Collect (position, shuffle_position) pairs for all rows in insertion order.
	allMatch := true
	for _, id := range ids {
		var pos, sp int64
		if err := d.QueryRow(`SELECT position, shuffle_position FROM queue WHERE id=?`, id).Scan(&pos, &sp); err != nil {
			t.Fatalf("query row %d: %v", id, err)
		}
		if pos != sp {
			allMatch = false
		}
	}
	if allMatch {
		t.Error("shuffle_position values are identical to position values; expected a different ordering")
	}
}

// TestUpsertSession verifies that UpsertSession writes all session fields and
// that a second call overwrites the previous values.
func TestUpsertSession(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hashA", "/trackA")

	if err := w.UpsertSession("hashA", 12345, true, "all"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	var (
		hash       sql.NullString
		positionMs int64
		shuffle    int
		repeatMode string
	)
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if !hash.Valid || hash.String != "hashA" {
		t.Errorf("current_hash = %v; want hashA", hash)
	}
	if positionMs != 12345 {
		t.Errorf("position_ms = %d; want 12345", positionMs)
	}
	if shuffle != 1 {
		t.Errorf("shuffle = %d; want 1", shuffle)
	}
	if repeatMode != "all" {
		t.Errorf("repeat_mode = %q; want all", repeatMode)
	}

	// Second call overwrites with new values.
	if err := w.UpsertSession("", 0, false, "none"); err != nil {
		t.Fatalf("UpsertSession (second): %v", err)
	}
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session after second call: %v", err)
	}
	if hash.Valid {
		t.Errorf("current_hash = %v; want NULL", hash)
	}
	if positionMs != 0 {
		t.Errorf("position_ms = %d; want 0", positionMs)
	}
	if shuffle != 0 {
		t.Errorf("shuffle = %d; want 0", shuffle)
	}
	if repeatMode != "none" {
		t.Errorf("repeat_mode = %q; want none", repeatMode)
	}
}

// TestUpsertSession_PropagatesError verifies that a DB error (e.g. invalid
// repeat_mode value violating the CHECK constraint) is returned to the caller.
func TestUpsertSession_PropagatesError(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	err := w.UpsertSession("", 0, false, "invalid_mode")
	if err == nil {
		t.Fatal("expected error for invalid repeat_mode CHECK violation, got nil")
	}
}

// TestUpdateSessionPosition verifies that UpdateSessionPosition updates only
// current_hash and position_ms, leaving shuffle and repeat_mode unchanged.
func TestUpdateSessionPosition(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hashB", "/trackB")

	// Set up a known state first.
	if err := w.UpsertSession("hashB", 1000, true, "one"); err != nil {
		t.Fatalf("UpsertSession setup: %v", err)
	}

	if err := w.UpdateSessionPosition("hashB", 9999); err != nil {
		t.Fatalf("UpdateSessionPosition: %v", err)
	}

	var (
		hash       sql.NullString
		positionMs int64
		shuffle    int
		repeatMode string
	)
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if !hash.Valid || hash.String != "hashB" {
		t.Errorf("current_hash = %v; want hashB", hash)
	}
	if positionMs != 9999 {
		t.Errorf("position_ms = %d; want 9999", positionMs)
	}
	// shuffle and repeat_mode must be untouched.
	if shuffle != 1 {
		t.Errorf("shuffle = %d; want 1 (unchanged)", shuffle)
	}
	if repeatMode != "one" {
		t.Errorf("repeat_mode = %q; want one (unchanged)", repeatMode)
	}
}

// TestUpdateSessionPosition_NullHash verifies that an empty hash stores NULL.
func TestUpdateSessionPosition_NullHash(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	if err := w.UpdateSessionPosition("", 500); err != nil {
		t.Fatalf("UpdateSessionPosition: %v", err)
	}

	var hash sql.NullString
	if err := d.QueryRow(`SELECT current_hash FROM session WHERE id=1`).Scan(&hash); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if hash.Valid {
		t.Errorf("current_hash = %v; want NULL for empty hash arg", hash)
	}
}

// TestWriteRepeatMode_ValidModes verifies that WriteRepeatMode accepts the
// supported values and persists them to the singleton session row.
func TestWriteRepeatMode_ValidModes(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	for _, mode := range []string{"none", "all", "one"} {
		if err := w.WriteRepeatMode(mode); err != nil {
			t.Fatalf("WriteRepeatMode(%q): %v", mode, err)
		}
		var got string
		if err := d.QueryRow(`SELECT repeat_mode FROM session WHERE id=1`).Scan(&got); err != nil {
			t.Fatalf("select repeat_mode: %v", err)
		}
		if got != mode {
			t.Errorf("repeat_mode = %q, want %q", got, mode)
		}
	}
}

// TestWriteVolume verifies that volume percentages are persisted as fractional
// values in the session row.
func TestWriteVolume(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	for _, pct := range []int{0, 50, 100, 130} {
		if err := w.WriteVolume(pct); err != nil {
			t.Fatalf("WriteVolume(%d): %v", pct, err)
		}
		var got float64
		if err := d.QueryRow(`SELECT volume FROM session WHERE id=1`).Scan(&got); err != nil {
			t.Fatalf("select volume: %v", err)
		}
		want := float64(pct) / 100.0
		if got != want {
			t.Errorf("volume = %v, want %v (pct=%d)", got, want, pct)
		}
	}
}
