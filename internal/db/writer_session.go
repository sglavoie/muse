package db

import (
	"database/sql"
	"math/rand/v2"
	"time"
)

type queueShuffleRow struct {
	id              int64
	position        int64
	shufflePosition int64
}

// WriteShuffle persists the shuffle flag to the session row and, when enabled
// is true, regenerates shuffle_position for all active queue rows with fresh
// random gapped integers inside a single transaction. When disabled, only the
// session flag is updated.
//
// shuffle_position is a persistence-only field. Playback randomisation is
// handled by mpv's native shuffle property (SetShuffle on the player). DB reads
// always use ORDER BY position; shuffle_position is regenerated on each enable
// (and on session restore when shuffle is on) to ensure varied playback order
// across sessions without requiring a separate read path.
func (w *Writer) WriteShuffle(enabled bool) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		shuffleVal := 0
		if enabled {
			shuffleVal = 1
		}
		if _, err := tx.Exec(
			`UPDATE session SET shuffle=?, updated_at=? WHERE id=1`,
			shuffleVal, time.Now().Unix(),
		); err != nil {
			return err
		}

		if enabled {
			rows, err := tx.Query(`SELECT id, position, shuffle_position FROM queue WHERE deleted_at IS NULL ORDER BY position`)
			if err != nil {
				return err
			}
			var rowsToShuffle []queueShuffleRow
			for rows.Next() {
				var row queueShuffleRow
				if err := rows.Scan(&row.id, &row.position, &row.shufflePosition); err != nil {
					rows.Close()
					return err
				}
				rowsToShuffle = append(rowsToShuffle, row)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}

			if len(rowsToShuffle) > 0 {
				perm := rand.Perm(len(rowsToShuffle))
				if len(rowsToShuffle) > 1 && shufflePermutationMatches(rowsToShuffle, perm) {
					perm[0], perm[1] = perm[1], perm[0]
				}
				for i, row := range rowsToShuffle {
					newPos := int64(perm[i]+1) * queuePositionGap
					if _, err := tx.Exec(
						`UPDATE queue SET shuffle_position=? WHERE id=?`,
						newPos, row.id,
					); err != nil {
						return err
					}
				}
			}
		}

		return tx.Commit()
	})
}

func shufflePermutationMatches(rows []queueShuffleRow, perm []int) bool {
	matchesQueueOrder := true
	matchesPreviousShuffle := true
	hasPreviousShuffle := true
	for i, row := range rows {
		newPos := int64(perm[i]+1) * queuePositionGap
		if newPos != row.position {
			matchesQueueOrder = false
		}
		if row.shufflePosition == 0 || newPos != row.shufflePosition {
			matchesPreviousShuffle = false
		}
		if row.shufflePosition == 0 {
			hasPreviousShuffle = false
		}
	}
	return matchesQueueOrder || (hasPreviousShuffle && matchesPreviousShuffle)
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
