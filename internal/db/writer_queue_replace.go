package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// ReplaceQueue commits removal and insertion together. The expected active IDs
// prevent an enqueue awaiting its UI acknowledgement from being silently lost.
func (w *Writer) ReplaceQueue(expected []int64, tracks []library.Track, source string) ([]int64, error) {
	var ids []int64
	err := w.Exec(func(database *sql.DB) error {
		tx, err := database.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		if err := requireActiveQueue(tx, expected); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE queue SET deleted_at=? WHERE deleted_at IS NULL`, time.Now().Unix()); err != nil {
			return err
		}
		rows := make([]queueInsert, len(tracks))
		for i, track := range tracks {
			rows[i] = queueInsert{hash: track.Hash, source: source}
		}
		inserted, err := appendQueueRows(tx, rows)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		ids = inserted
		return nil
	})
	return ids, err
}

// UndoReplaceQueue restores original identities, provenance and ordering in one
// transaction. A missing tombstone refuses the entire restore.
func (w *Writer) UndoReplaceQueue(expected, restore []int64) error {
	return w.Exec(func(database *sql.DB) error {
		tx, err := database.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		if err := requireActiveQueue(tx, expected); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE queue SET deleted_at=? WHERE deleted_at IS NULL`, time.Now().Unix()); err != nil {
			return err
		}
		for _, id := range restore {
			result, err := tx.Exec(`UPDATE queue SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL AND`+notPendingClause, id)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("original queue entry %d is unavailable", id)
			}
		}
		if err := setGappedOrderTx(tx, "queue", "", nil, restore); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func requireActiveQueue(tx *sql.Tx, expected []int64) error {
	want := make(map[int64]struct{}, len(expected))
	for _, id := range expected {
		want[id] = struct{}{}
	}
	rows, err := tx.Query(`SELECT id FROM queue WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if _, ok := want[id]; !ok {
			return fmt.Errorf("queue changed; wait for pending actions and retry")
		}
		delete(want, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(want) != 0 {
		return fmt.Errorf("queue changed; wait for pending actions and retry")
	}
	return nil
}
