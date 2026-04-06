package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// AppendTrack inserts track at the end of the play queue using a gapped
// position (MAX(position)+1000) and returns the new row's ID. The track must
// already exist in the tracks table (i.e., it was previously scanned).
// Duplicate queue entries for the same track hash are accepted without error.
// sourcePlaylist identifies the playlist the track was enqueued from; pass ""
// for tracks enqueued individually (e.g. from the library view).
func (w *Writer) AppendTrack(track library.Track, sourcePlaylist string) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(fmt.Sprintf(`
			INSERT INTO queue (track_hash, position, added_at, source_playlist)
			VALUES (?, COALESCE((SELECT MAX(position) FROM queue), 0) + %d, ?, ?)`, queuePositionGap),
			track.Hash, now, sourcePlaylist,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// AppendTracks batch-inserts tracks at the end of the play queue inside a
// single transaction. Each row gets source_playlist set to sourcePlaylist.
// Returns a slice of new row IDs in the same order as the input tracks.
func (w *Writer) AppendTracks(tracks []library.Track, sourcePlaylist string) ([]int64, error) {
	if len(tracks) == 0 {
		return nil, nil
	}
	var ids []int64
	err := w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var maxPos int64
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), 0) FROM queue`,
		).Scan(&maxPos); err != nil {
			return err
		}

		now := time.Now().Unix()
		for _, t := range tracks {
			maxPos += queuePositionGap
			res, err := tx.Exec(
				`INSERT INTO queue (track_hash, position, added_at, source_playlist) VALUES (?, ?, ?, ?)`,
				t.Hash, maxPos, now, sourcePlaylist,
			)
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return tx.Commit()
	})
	return ids, err
}

// DequeueTracksSoft batch soft-deletes all queue entries identified by ids
// by setting their deleted_at timestamp inside a single transaction.
func (w *Writer) DequeueTracksSoft(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, id := range ids {
			if _, err := tx.Exec(`UPDATE queue SET deleted_at=? WHERE id=?`, now, id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// MarkQueueEntryMissing sets missing=1 on the queue row identified by id.
// The entry is kept in the queue so it remains visible (grayed out) in the UI.
func (w *Writer) MarkQueueEntryMissing(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE queue SET missing=1 WHERE id=?`, id)
		return err
	})
}

// ClearQueueEntriesMissing sets missing=0 on all queue rows that reference
// trackHash. Call this after ClearTrackMissing during rescan so that the UI
// removes the grayed-out state from every queued instance of the track.
func (w *Writer) ClearQueueEntriesMissing(trackHash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE queue SET missing=0 WHERE track_hash=?`, trackHash)
		return err
	})
}

// DequeueTrackSoft marks the queue entry identified by id as soft-deleted by
// setting its deleted_at timestamp to the current Unix time. The row is
// retained in the database and can be restored via UndoDequeue within the
// undo window.
func (w *Writer) DequeueTrackSoft(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE queue SET deleted_at=? WHERE id=?`, time.Now().Unix(), id)
		return err
	})
}

// UndoDequeue clears the deleted_at flag on the queue entry identified by id,
// restoring it to the active set. Call this within the undo window to reverse
// a DequeueTrackSoft.
func (w *Writer) UndoDequeue(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE queue SET deleted_at=NULL WHERE id=?`, id)
		return err
	})
}

// RenumberQueuePositions rewrites the position column for all active queue
// rows (deleted_at IS NULL) using sequential gaps of 1000 (1000, 2000, 3000…)
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

// ClearQueueSoft soft-deletes every active queue row (deleted_at IS NULL) by
// setting deleted_at to the current Unix time. It returns the IDs of all rows
// that were marked so the caller can pass them to UndoClearQueue within the
// undo window. Already soft-deleted rows are left untouched.
func (w *Writer) ClearQueueSoft() ([]int64, error) {
	var ids []int64
	err := w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		rows, err := tx.Query(`SELECT id FROM queue WHERE deleted_at IS NULL`)
		if err != nil {
			return err
		}
		var collected []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			collected = append(collected, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if len(collected) == 0 {
			return tx.Commit()
		}

		now := time.Now().Unix()
		if _, err = tx.Exec(`UPDATE queue SET deleted_at=? WHERE deleted_at IS NULL`, now); err != nil {
			return err
		}

		ids = collected
		return tx.Commit()
	})
	return ids, err
}

// UndoClearQueue clears deleted_at on every queue row whose ID appears in ids,
// restoring those rows to the active set. All updates execute inside a single
// transaction so either all rows are restored or none are.
func (w *Writer) UndoClearQueue(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		for _, id := range ids {
			if _, err := tx.Exec(`UPDATE queue SET deleted_at=NULL WHERE id=?`, id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// PurgeDeletedQueueEntries physically removes all queue rows whose deleted_at
// is non-NULL and strictly less than the before Unix timestamp. Call this once
// the undo window has expired to reclaim space.
func (w *Writer) PurgeDeletedQueueEntries(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM queue WHERE deleted_at IS NOT NULL AND deleted_at < ?`, before,
		)
		return err
	})
}

// SetQueueOrder assigns sequential positions (1000, 2000, 3000…) to the active
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

		n := int64(len(ids))
		maxNewPos := n * queuePositionGap

		// Find the global maximum position across all rows (including soft-deleted)
		// so we can compute a safe relocation zone above it.
		var globalMax int64
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), 0) FROM queue`,
		).Scan(&globalMax); err != nil {
			return err
		}

		// Collect soft-deleted rows whose positions fall within [queuePositionGap,
		// maxNewPos] and are exact multiples of queuePositionGap — these would
		// conflict with the new sequential positions.
		type qrow struct{ id, position int64 }
		rows, err := tx.Query(
			`SELECT id, position FROM queue
			 WHERE deleted_at IS NOT NULL
			   AND position >= ?
			   AND position <= ?`,
			queuePositionGap, maxNewPos,
		)
		if err != nil {
			return err
		}
		var conflicting []qrow
		for rows.Next() {
			var r qrow
			if err := rows.Scan(&r.id, &r.position); err != nil {
				rows.Close()
				return err
			}
			if r.position%queuePositionGap == 0 {
				conflicting = append(conflicting, r)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// Phase 1: Move all target rows to a temporary zone strictly above
		// globalMax. This clears the 1000…N*1000 range of any potential
		// collisions between active rows regardless of their current positions.
		safeBase := max(globalMax, maxNewPos)

		// Move conflicting soft-deleted rows first (they stay above safeBase).
		for i, r := range conflicting {
			safePos := safeBase + int64(i+1)*queuePositionGap
			if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, safePos, r.id); err != nil {
				return err
			}
		}

		// Move all target active rows to a temporary zone above the safe base so
		// that phase 2 assignments never collide with pre-existing positions.
		tempBase := safeBase + int64(len(conflicting)+1)*queuePositionGap
		for i, id := range ids {
			tmpPos := tempBase + int64(i)*queuePositionGap
			if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, tmpPos, id); err != nil {
				return err
			}
		}

		// Phase 2: Assign final positions 1000, 2000, …, N*1000 in the given
		// order. The range is now free of conflicts.
		for i, id := range ids {
			pos := int64(i+1) * queuePositionGap
			if _, err := tx.Exec(`UPDATE queue SET position=? WHERE id=?`, pos, id); err != nil {
				return err
			}
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
