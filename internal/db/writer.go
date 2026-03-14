package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const writerDrainTimeout = 5 * time.Second

// writeCmd is a single unit of work sent to the writer goroutine.
// The caller blocks on resp until the goroutine sends the result back.
type writeCmd struct {
	fn   func(*sql.DB) error
	resp chan error
}

// Writer serializes all DB mutations through a single goroutine, eliminating
// SQLITE_BUSY contention. Callers submit work via Exec and block on a
// per-request response channel until the goroutine completes the write.
type Writer struct {
	db   *sql.DB
	cmds chan writeCmd
	done chan struct{}
}

// NewWriter creates a Writer and starts its background goroutine. The goroutine
// runs until ctx is cancelled. The caller should call Wait after cancelling ctx
// to ensure a clean shutdown.
func NewWriter(ctx context.Context, db *sql.DB) *Writer {
	w := &Writer{
		db:   db,
		cmds: make(chan writeCmd, 64),
		done: make(chan struct{}),
	}
	go w.run(ctx)
	return w
}

// Exec submits fn to the writer goroutine and blocks until it completes.
// fn receives the underlying *sql.DB and should perform exactly one logical
// write (INSERT, UPDATE, DELETE, or a transaction wrapping several).
// Returns an error if fn fails or if the writer has already stopped.
func (w *Writer) Exec(fn func(*sql.DB) error) error {
	resp := make(chan error, 1)
	cmd := writeCmd{fn: fn, resp: resp}

	select {
	case w.cmds <- cmd:
	case <-w.done:
		return fmt.Errorf("db writer: stopped")
	}

	select {
	case err := <-resp:
		return err
	case <-w.done:
		return fmt.Errorf("db writer: stopped while waiting for result")
	}
}

// Wait blocks until the writer goroutine has fully exited. Call this after
// cancelling the context passed to NewWriter to ensure a clean shutdown.
func (w *Writer) Wait() {
	<-w.done
}

// run is the single writer goroutine. It drains cmds until ctx is cancelled,
// then flushes any buffered commands before closing done.
func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case cmd := <-w.cmds:
			cmd.resp <- cmd.fn(w.db)
		case <-ctx.Done():
			w.drain()
			return
		}
	}
}

// AppendTrack inserts track at the end of the play queue using a gapped
// position (MAX(position)+1000) and returns the new row's ID. The track must
// already exist in the tracks table (i.e., it was previously scanned).
// Duplicate queue entries for the same track hash are accepted without error.
func (w *Writer) AppendTrack(track library.Track) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(`
			INSERT INTO queue (track_hash, position, added_at)
			VALUES (?, COALESCE((SELECT MAX(position) FROM queue), 0) + 1000, ?)`,
			track.Hash, now,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// MarkTrackMissing sets missing=1 on the tracks row identified by hash and
// updates its updated_at timestamp. It is a no-op if no row matches.
func (w *Writer) MarkTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=1, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
	})
}

// ClearTrackMissing sets missing=0 on the tracks row identified by hash and
// updates its updated_at timestamp. Call this during rescan when a file with a
// matching BLAKE3 hash is located again.
func (w *Writer) ClearTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=0, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
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
		defer tx.Rollback() //nolint:errcheck

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

		// Update in reverse order (highest new position first) to avoid
		// transient unique-constraint violations during intermediate states.
		for i := len(active) - 1; i >= 0; i-- {
			newPos := int64(i+1) * 1000
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
		defer tx.Rollback() //nolint:errcheck

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
		defer tx.Rollback() //nolint:errcheck

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

// WriteShuffle persists the shuffle flag to the session row and, when enabled
// is true, regenerates shuffle_position for all active queue rows with fresh
// random gapped integers inside a single transaction. When disabled, only the
// session flag is updated; the caller switches ordering back to the position
// column at query time.
func (w *Writer) WriteShuffle(enabled bool) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck

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
			rows, err := tx.Query(`SELECT id FROM queue WHERE deleted_at IS NULL`)
			if err != nil {
				return err
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}

			if len(ids) > 0 {
				perm := rand.Perm(len(ids))
				for i, id := range ids {
					newPos := int64(perm[i]+1) * 1000
					if _, err := tx.Exec(
						`UPDATE queue SET shuffle_position=? WHERE id=?`,
						newPos, id,
					); err != nil {
						return err
					}
				}
			}
		}

		return tx.Commit()
	})
}

// WriteRepeatMode persists the repeat mode to the single session row.
// mode must be one of "none", "all", or "one" (matching the DB CHECK constraint).
func (w *Writer) WriteRepeatMode(mode string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE session SET repeat_mode=?, updated_at=? WHERE id=1`,
			mode, time.Now().Unix(),
		)
		return err
	})
}

// drain flushes all commands already buffered in the cmds channel after ctx is
// cancelled. A timeout guards against a single stuck write blocking shutdown
// indefinitely. New Exec calls after drain returns will see w.done closed and
// receive an error immediately.
func (w *Writer) drain() {
	deadline := time.Now().Add(writerDrainTimeout)
	for time.Now().Before(deadline) {
		select {
		case cmd := <-w.cmds:
			cmd.resp <- cmd.fn(w.db)
		default:
			return
		}
	}
}
