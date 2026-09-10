package db

import (
	"database/sql"
	"time"
)

// notPendingClause matches queue rows that are not part of an unaccepted
// preparation. COALESCE folds an active row's NULL onto 0 so the comparison is
// true for it as well.
//
// Every soft-delete, undo, and purge statement carries it: a prepared batch is
// not a tombstone, and letting an ordinary queue operation clear or resurrect
// its marker would publish tracks the user never accepted.
// The marker is spelled out as a literal rather than bound so the clause can be
// appended to any statement without every caller threading an extra argument
// into the right slot. The assertion below keeps the two in sync.
const notPendingClause = ` COALESCE(deleted_at, 0) != 9223372036854775807`

// This unsigned conversion fails to compile if pendingQueueDeletedAt ever stops
// matching the literal inlined into notPendingClause, which would otherwise
// silently turn every guard above into a no-op.
const _ = uint(pendingQueueDeletedAt - 9223372036854775807)

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
			if _, err := tx.Exec(
				`UPDATE queue SET deleted_at=? WHERE id=? AND`+notPendingClause, now, id,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// DequeueTrackSoft marks the queue entry identified by id as soft-deleted by
// setting its deleted_at timestamp to the current Unix time. The row is
// retained in the database and can be restored via UndoDequeue within the
// undo window.
//
// Provenance is left untouched, so a restored row is still recognised as the
// DJ's or the user's.
func (w *Writer) DequeueTrackSoft(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE queue SET deleted_at=? WHERE id=? AND`+notPendingClause,
			time.Now().Unix(), id,
		)
		return err
	})
}

// UndoDequeue clears the deleted_at flag on the queue entry identified by id,
// restoring it to the active set. Call this within the undo window to reverse
// a DequeueTrackSoft.
//
// The row comes back exactly as it was, provenance included: a DJ selection
// restored by undo is still DJ-owned and still carries the reasons it was
// picked. A row belonging to an unaccepted preparation is never restored.
func (w *Writer) UndoDequeue(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE queue SET deleted_at=NULL WHERE id=? AND`+notPendingClause, id)
		return err
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

		if err := tx.Commit(); err != nil {
			return err
		}
		ids = collected
		return nil
	})
	return ids, err
}

// UndoClearQueue clears deleted_at on every queue row whose ID appears in ids,
// restoring those rows to the active set. All updates execute inside a single
// transaction so either all rows are restored or none are. Each row keeps the
// provenance it had before the clear.
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
			if _, err := tx.Exec(
				`UPDATE queue SET deleted_at=NULL WHERE id=? AND`+notPendingClause, id,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// PurgeDeletedQueueEntries physically removes all queue rows whose deleted_at
// is non-NULL and strictly less than the before Unix timestamp. Call this once
// the undo window has expired to reclaim space. Provenance columns go with the
// row; nothing else references them.
//
// Rows belonging to an unaccepted preparation are excluded explicitly as well
// as by the timestamp comparison, so a caller passing an absurd before value
// still cannot delete a batch that is in flight.
func (w *Writer) PurgeDeletedQueueEntries(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM queue WHERE deleted_at IS NOT NULL AND deleted_at < ? AND`+notPendingClause,
			before,
		)
		return err
	})
}
