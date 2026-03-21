package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	// writerDrainTimeout is how long Close waits for in-flight writes to
	// finish before giving up. Keep it short so the app exits promptly.
	writerDrainTimeout = 5 * time.Second

	// writerChannelBuffer is the capacity of the async command channel.
	// 64 is large enough to absorb burst writes (e.g. batch enqueue)
	// without blocking the TUI goroutine.
	writerChannelBuffer = 64

	// queuePositionGap is the integer gap between consecutive queue
	// positions. Using 1000 instead of 1 allows single-row reorders
	// (move up/down) to be expressed as simple position swaps without
	// renumbering the entire queue.
	queuePositionGap = 1000
)

// Compile-time check: *Writer must satisfy the sessionWriter interface
// used by the TUI layer (WriteRepeatMode, WriteShuffle, UpdateSessionPosition,
// WriteVolume). This catches method-signature drift without requiring a runtime
// assignment.
var _ interface {
	WriteRepeatMode(mode string) error
	WriteShuffle(enabled bool) error
	UpdateSessionPosition(hash string, positionMs int64) error
	WriteVolume(volume int) error
} = (*Writer)(nil)

// Compile-time check: *Writer must implement UpsertBatch so that the
// scan-to-DB bridge in main.go and any future consumer cannot diverge from
// the actual method signature without a build failure.
var _ interface {
	UpsertBatch(tracks []library.Track) error
} = (*Writer)(nil)

// Compile-time check: *Writer must satisfy the queueDequeuer interface used
// by the TUI layer (DequeueTrackSoft, UndoDequeue, ClearQueueSoft,
// UndoClearQueue, PurgeDeletedQueueEntries). Mirrors the sessionWriter and
// UpsertBatch assertions above to catch method-signature drift at build time.
var _ interface {
	DequeueTrackSoft(id int64) error
	UndoDequeue(id int64) error
	ClearQueueSoft() ([]int64, error)
	UndoClearQueue(ids []int64) error
	PurgeDeletedQueueEntries(before int64) error
} = (*Writer)(nil)

// Compile-time check: *Writer must satisfy the favoriteWriter interface used
// by the TUI layer (AddFavorite, RemoveFavorite, RemoveFavoritesByTypeAndValues).
var _ interface {
	AddFavorite(typ, value string) error
	RemoveFavorite(typ, value string) error
	RemoveFavoritesByTypeAndValues(typ string, values []string) error
} = (*Writer)(nil)

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
		cmds: make(chan writeCmd, writerChannelBuffer),
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
		// The writer goroutine closed done after drain(). If drain
		// processed our command, resp already has the result. Check it
		// with a non-blocking receive to avoid a spurious "stopped"
		// error when the write actually succeeded.
		select {
		case err := <-resp:
			return err
		default:
			return fmt.Errorf("db writer: stopped while waiting for result")
		}
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
		// Move conflicting soft-deleted rows above the renumber range.
		for _, r := range conflicting {
			safePos := maxNewPos + r.position
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
					newPos := int64(perm[i]+1) * queuePositionGap
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
// volume is an integer percentage (0–130) that is converted to a float
// (0.0–1.3) for storage in the DB volume column.
func (w *Writer) WriteVolume(volume int) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE session SET volume = ?, updated_at = ? WHERE id = 1`,
			float64(volume)/100.0, time.Now().Unix(),
		)
		return err
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

// CreatePlaylist inserts a new playlist with the given name and returns its ID.
// The playlist is immediately active (deleted_at IS NULL).
func (w *Writer) CreatePlaylist(name string) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(
			`INSERT INTO playlists (name, created_at, updated_at) VALUES (?, ?, ?)`,
			name, now, now,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RenamePlaylist updates the name of the playlist identified by id.
// It is a no-op if no active playlist with that ID exists.
func (w *Writer) RenamePlaylist(id int64, name string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE playlists SET name=?, updated_at=? WHERE id=?`,
			name, time.Now().Unix(), id,
		)
		return err
	})
}

// DeletePlaylistSoft soft-deletes the playlist identified by id by setting its
// deleted_at timestamp. The row is retained for the undo window.
func (w *Writer) DeletePlaylistSoft(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE playlists SET deleted_at=? WHERE id=?`,
			time.Now().Unix(), id,
		)
		return err
	})
}

// UndoDeletePlaylist clears the deleted_at flag on the playlist identified by
// id, restoring it to the active set. Call this within the undo window to
// reverse a DeletePlaylistSoft.
func (w *Writer) UndoDeletePlaylist(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE playlists SET deleted_at=NULL WHERE id=?`, id)
		return err
	})
}

// PurgeDeletedPlaylists physically removes all playlist rows whose deleted_at
// is non-NULL and strictly less than the before Unix timestamp.
// Call this once the undo window has expired to reclaim space.
func (w *Writer) PurgeDeletedPlaylists(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM playlists WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
			before,
		)
		return err
	})
}

// AddTracksToPlaylist appends hashes to playlistID using gapped-integer
// positioning (MAX(position)+1000 per track). All inserts run inside a single
// transaction. It is a no-op when hashes is empty.
func (w *Writer) AddTracksToPlaylist(playlistID int64, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var maxPos int64
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), 0) FROM playlist_tracks WHERE playlist_id=?`,
			playlistID,
		).Scan(&maxPos); err != nil {
			return err
		}

		now := time.Now().Unix()
		for _, hash := range hashes {
			maxPos += queuePositionGap
			if _, err := tx.Exec(
				`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, ?, ?, ?)`,
				playlistID, hash, maxPos, now,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// RemovePlaylistTrackSoft soft-deletes the playlist_tracks row identified by
// id by setting its deleted_at timestamp. The row is retained for the undo
// window and can be restored via UndoRemovePlaylistTrack.
func (w *Writer) RemovePlaylistTrackSoft(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`,
			time.Now().Unix(), id,
		)
		return err
	})
}

// UndoRemovePlaylistTrack clears the deleted_at flag on the playlist_tracks
// row identified by id, restoring it to the active set. Call this within the
// undo window to reverse a RemovePlaylistTrackSoft.
func (w *Writer) UndoRemovePlaylistTrack(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE playlist_tracks SET deleted_at=NULL WHERE id=?`, id)
		return err
	})
}

// ReorderPlaylistTrack performs a single-step adjacent swap for the track
// identified by trackID inside playlistID. direction must be -1 (move up,
// toward lower position) or +1 (move down, toward higher position). It is a
// no-op when the track is already at the boundary. Only active
// (deleted_at IS NULL) neighbours are considered.
func (w *Writer) ReorderPlaylistTrack(playlistID, trackID int64, direction int) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var currPos int64
		if err := tx.QueryRow(
			`SELECT position FROM playlist_tracks WHERE id=? AND playlist_id=?`,
			trackID, playlistID,
		).Scan(&currPos); err != nil {
			return err
		}

		var adjQuery string
		if direction > 0 {
			adjQuery = `SELECT id, position FROM playlist_tracks
				WHERE playlist_id=? AND deleted_at IS NULL AND position > ?
				ORDER BY position ASC LIMIT 1`
		} else {
			adjQuery = `SELECT id, position FROM playlist_tracks
				WHERE playlist_id=? AND deleted_at IS NULL AND position < ?
				ORDER BY position DESC LIMIT 1`
		}

		var adjID, adjPos int64
		if err := tx.QueryRow(adjQuery, playlistID, currPos).Scan(&adjID, &adjPos); err != nil {
			if err == sql.ErrNoRows {
				return nil // already at boundary
			}
			return err
		}

		// Three-step swap to avoid transient unique-constraint violations.
		// Negative positions cannot exist in normal use so -1 is a safe temp.
		const tempPos int64 = -1
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, tempPos, trackID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, currPos, adjID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, adjPos, trackID); err != nil {
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
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var currPos int64
		if err := tx.QueryRow(
			`SELECT position FROM queue WHERE id=?`,
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

// PurgeDeletedPlaylistTracks physically removes all playlist_tracks rows whose
// deleted_at is non-NULL and strictly less than the before Unix timestamp.
// Call this once the undo window has expired to reclaim space.
func (w *Writer) PurgeDeletedPlaylistTracks(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM playlist_tracks WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
			before,
		)
		return err
	})
}

// UpdateTrackMetadata updates the title, artist, and album for the track
// identified by hash and sets user_edited=1 so that future rescans preserve
// the manually edited metadata.
func (w *Writer) UpdateTrackMetadata(hash, title, artist, album string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET title=?, artist=?, album=?, user_edited=1, updated_at=? WHERE hash=?`,
			title, artist, album, time.Now().Unix(), hash,
		)
		return err
	})
}

// UpsertBatch writes tracks into the tracks table inside a single transaction
// using an INSERT … ON CONFLICT(hash) DO UPDATE so that existing rows are
// updated in place without cascading deletes on their FK dependants (queue,
// playlist_tracks). created_at is preserved on conflict; all other fields are
// updated to the values supplied by the scanner. Duration is converted from
// seconds (float64) to milliseconds (int64).
// UpsertBatch is a no-op when tracks is empty.
func (w *Writer) UpsertBatch(tracks []library.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, t := range tracks {
			durationMs := int64(t.Duration * 1000)
			_, err := tx.Exec(`
				INSERT INTO tracks (hash, path, title, artist, album, duration_ms, size_bytes, mod_time, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					path        = excluded.path,
					title       = CASE WHEN tracks.user_edited = 0 THEN excluded.title ELSE tracks.title END,
					artist      = CASE WHEN tracks.user_edited = 0 THEN excluded.artist ELSE tracks.artist END,
					album       = CASE WHEN tracks.user_edited = 0 THEN excluded.album ELSE tracks.album END,
					duration_ms = excluded.duration_ms,
					size_bytes  = excluded.size_bytes,
					mod_time    = excluded.mod_time,
					missing     = 0,
					updated_at  = excluded.updated_at`,
				t.Hash, t.Path, t.Title, t.Artist, t.Album, durationMs, t.SizeBytes, t.ModTime, now, now,
			)
			if err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// AddFavorite inserts a favorite with the given type and value. The operation
// is idempotent: if the (type, value) pair already exists the insert is ignored.
func (w *Writer) AddFavorite(typ, value string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO favorites (type, value, created_at) VALUES (?, ?, ?)`,
			typ, value, time.Now().Unix(),
		)
		return err
	})
}

// RemoveFavorite deletes the favorite identified by (type, value). It is a
// no-op when no matching row exists.
func (w *Writer) RemoveFavorite(typ, value string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`DELETE FROM favorites WHERE type = ? AND value = ?`, typ, value)
		return err
	})
}

// RemoveFavoritesByTypeAndValues deletes all favorites of the given type whose
// value appears in values. All deletes run inside a single transaction.
// It is a no-op when values is empty.
func (w *Writer) RemoveFavoritesByTypeAndValues(typ string, values []string) error {
	if len(values) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		for _, v := range values {
			if _, err := tx.Exec(`DELETE FROM favorites WHERE type = ? AND value = ?`, typ, v); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// rollback attempts to roll back tx, logging unexpected errors. The common
// sql.ErrTxDone (commit already succeeded) is silently ignored.
func rollback(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Warn("tx rollback failed", "err", err)
	}
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
