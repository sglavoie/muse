package db

import (
	"database/sql"
	"time"
)

// WriteShuffle persists the shuffle flag to the singleton session row so it can
// be restored across restarts. Shuffle playback order is Go-owned (the player's
// in-memory traversal state); the database only records whether shuffle is on.
//
// The legacy shuffle_position column is retained for schema compatibility but is
// no longer read or regenerated — playback randomisation happens entirely in the
// player, and shuffle history is intentionally not persisted across restarts.
func (w *Writer) WriteShuffle(enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	return w.updateSessionField("shuffle", v)
}

// UpsertSession updates all session state fields atomically in a single UPDATE:
// current_hash (nullable), position_ms, shuffle, and repeat_mode. This method
// is not called from the hot-path (individual Write* methods handle each field);
// it is retained for use in tests as a convenient setup helper that establishes
// full session state in one call.
func (w *Writer) UpsertSession(hash string, positionMs int64, shuffle bool, repeatMode string) error {
	return w.Exec(func(db *sql.DB) error {
		shuffleVal := 0
		if shuffle {
			shuffleVal = 1
		}
		var hashArg any
		if hash != "" {
			hashArg = hash
		}
		_, err := db.Exec(
			`UPDATE session SET current_hash=?, position_ms=?, shuffle=?, repeat_mode=?, updated_at=? WHERE id=1`,
			hashArg, positionMs, shuffleVal, repeatMode, time.Now().Unix(),
		)
		return err
	})
}

// UpdateSessionPosition updates only current_hash and position_ms on the
// single session row. Use this for the periodic 10-second ticker writes where
// only elapsed position needs persisting, avoiding unnecessary column churn.
func (w *Writer) UpdateSessionPosition(hash string, positionMs int64) error {
	return w.Exec(func(db *sql.DB) error {
		var hashArg any
		if hash != "" {
			hashArg = hash
		}
		_, err := db.Exec(
			`UPDATE session SET current_hash=?, position_ms=?, updated_at=? WHERE id=1`,
			hashArg, positionMs, time.Now().Unix(),
		)
		return err
	})
}

// WriteVolume persists the volume percentage to the single session row.
// volume is an integer percentage (0–100) that is converted to a float
// (0.0–1.0) for storage in the DB volume column.
func (w *Writer) WriteVolume(volume int) error {
	return w.updateSessionField("volume", float64(volume)/100.0)
}

// WriteRadioMode persists the radio mode flag to the single session row.
func (w *Writer) WriteRadioMode(enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	return w.updateSessionField("radio_mode", v)
}

// WriteRepeatMode persists the repeat mode to the single session row.
// mode must be one of "none", "all", or "one" (matching the DB CHECK constraint).
func (w *Writer) WriteRepeatMode(mode string) error {
	return w.updateSessionField("repeat_mode", mode)
}

// updateSessionField sets a single named column on the session row (id=1)
// together with updated_at in one atomic Exec.
func (w *Writer) updateSessionField(col string, val any) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE session SET `+col+`=?, updated_at=? WHERE id=1`, //nolint:gosec // col is a hardcoded internal column name, not user input
			val, time.Now().Unix(),
		)
		return err
	})
}
