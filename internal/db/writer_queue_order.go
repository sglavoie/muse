package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// setGappedOrderTx assigns sequential positions (gap, 2*gap, ...) to the rows
// identified by ids inside tx. It handles three phases:
//  1. Relocate soft-deleted rows that occupy positions in the new range (to
//     avoid unique-constraint violations from the unique position index).
//  2. Move all target rows to a temporary zone above the global maximum so that
//     phase 3 assignments never collide with any pre-existing position.
//  3. Assign the final positions gap, 2*gap, ..., N*gap in the given order.
//
// table is the SQL table name (e.g. "queue" or "playlist_tracks").
// scopeFilter is an optional extra WHERE fragment appended to every query
// (e.g. "AND playlist_id=?"); scopeArgs holds its bound arguments, which are
// appended after any inline positional arguments. Pass nil for an unscoped
// table like queue.
func setGappedOrderTx(tx *sql.Tx, table, scopeFilter string, scopeArgs []any, ids []int64) error {
	n := int64(len(ids))
	maxNewPos := n * queuePositionGap

	withScope := func(base ...any) []any {
		return append(base, scopeArgs...)
	}

	var globalMax int64
	if err := tx.QueryRow(
		fmt.Sprintf(`SELECT COALESCE(MAX(position), 0) FROM %s WHERE 1=1 %s`, table, scopeFilter),
		withScope()...,
	).Scan(&globalMax); err != nil {
		return err
	}

	type row struct{ id, position int64 }
	softRows, err := tx.Query(
		fmt.Sprintf(
			`SELECT id, position FROM %s WHERE deleted_at IS NOT NULL AND position >= ? AND position <= ? %s`,
			table, scopeFilter,
		),
		withScope(queuePositionGap, maxNewPos)...,
	)
	if err != nil {
		return err
	}
	var conflicting []row
	for softRows.Next() {
		var r row
		if err := softRows.Scan(&r.id, &r.position); err != nil {
			softRows.Close()
			return err
		}
		if r.position%queuePositionGap == 0 {
			conflicting = append(conflicting, r)
		}
	}
	softRows.Close()
	if err := softRows.Err(); err != nil {
		return err
	}

	safeBase := max(globalMax, maxNewPos)

	for i, r := range conflicting {
		safePos := safeBase + int64(i+1)*queuePositionGap
		if _, err := tx.Exec(
			fmt.Sprintf(`UPDATE %s SET position=? WHERE id=?`, table),
			safePos, r.id,
		); err != nil {
			return err
		}
	}

	tempBase := safeBase + int64(len(conflicting)+1)*queuePositionGap
	for i, id := range ids {
		tmpPos := tempBase + int64(i)*queuePositionGap
		if _, err := tx.Exec(
			fmt.Sprintf(`UPDATE %s SET position=? WHERE id=?`, table),
			tmpPos, id,
		); err != nil {
			return err
		}
	}

	for i, id := range ids {
		pos := int64(i+1) * queuePositionGap
		if _, err := tx.Exec(
			fmt.Sprintf(`UPDATE %s SET position=? WHERE id=?`, table),
			pos, id,
		); err != nil {
			return err
		}
	}
	return nil
}

// RenumberQueuePositions rewrites the position column for all active queue
// rows (deleted_at IS NULL) using sequential gaps of 1000 (1000, 2000, 3000...)
// inside a single transaction. It is a no-op when no adjacent pair of active
// rows has a gap below 1.
func (w *Writer) RenumberQueuePositions() error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		type qrow struct {
			id       int64
			position int64
		}

		rows, err := tx.Query(
			`SELECT id, position FROM queue WHERE deleted_at IS NULL ORDER BY position`,
		)
		if err != nil {
			return err
		}
		var active []qrow
		for rows.Next() {
			var r qrow
			if err := rows.Scan(&r.id, &r.position); err != nil {
				rows.Close()
				return err
			}
			active = append(active, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		needsRenumber := false
		for i := 1; i < len(active); i++ {
			if active[i].position-active[i-1].position < 1 {
				needsRenumber = true
				break
			}
		}
		if !needsRenumber {
			return nil
		}

		// Relocate soft-deleted rows whose positions would collide with the
		// new gapped sequence. The queue_pos_idx unique constraint covers ALL
		// rows (including soft-deleted), so any soft-deleted row at a position
		// in the range [1000, len(active)*1000] must be moved first.
		maxNewPos := int64(len(active)) * queuePositionGap

		// Find the highest position across ALL rows so we can place
		// relocated soft-deleted rows safely above every existing position.
		var globalMax int64
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), 0) FROM queue`,
		).Scan(&globalMax); err != nil {
			return err
		}

		softRows, err := tx.Query(
			`SELECT id, position FROM queue WHERE deleted_at IS NOT NULL AND position >= 1000 AND position <= ?`,
			maxNewPos,
		)
		if err != nil {
			return err
		}
		var conflicting []qrow
		for softRows.Next() {
			var r qrow
			if err := softRows.Scan(&r.id, &r.position); err != nil {
				softRows.Close()
				return err
			}
			if r.position%queuePositionGap == 0 {
				conflicting = append(conflicting, r)
			}
		}
		softRows.Close()
		if err := softRows.Err(); err != nil {
			return err
		}
		// Move conflicting soft-deleted rows above every existing position
		// to guarantee no collisions with other soft-deleted rows.
		safeBase := max(globalMax, maxNewPos)
		for i, r := range conflicting {
			safePos := safeBase + int64(i+1)*queuePositionGap
			if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, safePos, r.id); err != nil {
				return err
			}
		}

		// Update in reverse order (highest new position first) to avoid
		// transient unique-constraint violations during intermediate states.
		for i := len(active) - 1; i >= 0; i-- {
			newPos := int64(i+1) * queuePositionGap
			if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, newPos, active[i].id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// SetQueueOrder assigns sequential positions (1000, 2000, 3000...) to the active
// queue rows in the order given by ids. Any soft-deleted rows whose positions
// would collide with the new range are relocated to a safe zone above the
// current global maximum first. Active rows not listed in ids are untouched.
func (w *Writer) SetQueueOrder(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		if err := setGappedOrderTx(tx, "queue", "", nil, ids); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// ReorderQueueTrack performs a single-step adjacent swap for the queue entry
// identified by trackID. direction must be -1 (move up, toward lower position)
// or +1 (move down, toward higher position). It is a no-op when the track is
// already at the boundary. Only active (deleted_at IS NULL) neighbours are
// considered.
func (w *Writer) ReorderQueueTrack(trackID int64, direction int) error {
	if direction != -1 && direction != 1 {
		return errors.New("direction must be -1 or +1")
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var currPos int64
		if err := tx.QueryRow(
			`SELECT position FROM queue WHERE id=? AND deleted_at IS NULL`,
			trackID,
		).Scan(&currPos); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Queue entry was concurrently dequeued or never existed;
				// treat as a no-op so a stale UI ID does not error out.
				return nil
			}
			return err
		}

		var adjQuery string
		if direction > 0 {
			adjQuery = `SELECT id, position FROM queue
				WHERE deleted_at IS NULL AND position > ?
				ORDER BY position ASC LIMIT 1`
		} else {
			adjQuery = `SELECT id, position FROM queue
				WHERE deleted_at IS NULL AND position < ?
				ORDER BY position DESC LIMIT 1`
		}

		var adjID, adjPos int64
		if err := tx.QueryRow(adjQuery, currPos).Scan(&adjID, &adjPos); err != nil {
			if err == sql.ErrNoRows {
				return nil // already at boundary
			}
			return err
		}

		// Three-step swap to avoid transient unique-constraint violations.
		// Negative positions cannot exist in normal use so -1 is a safe temp.
		const tempPos int64 = -1
		if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, tempPos, trackID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, currPos, adjID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, adjPos, trackID); err != nil {
			return err
		}
		return tx.Commit()
	})
}
