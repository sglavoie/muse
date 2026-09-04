package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// pendingRadioDeletedAt is a durable marker for a Radio batch whose scoring
// command committed but whose result has not yet been accepted by the TUI.
// It is deliberately later than any real Unix timestamp, so ordinary
// soft-delete purges cannot remove an in-flight batch before it is activated.
const pendingRadioDeletedAt int64 = 1<<63 - 1

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
		return make([]int64, 0), nil
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
		collected := make([]int64, 0, len(tracks))
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
			collected = append(collected, id)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		ids = collected
		return nil
	})
	return ids, err
}

// AppendRadioTracksPending persists a Radio batch without making it visible to
// ReadQueue. The TUI activates the exact IDs only after confirming the async
// result still belongs to the current Radio request. A crash before that point
// therefore cannot restore a discarded batch on the next launch.
func (w *Writer) AppendRadioTracksPending(tracks []library.Track) ([]int64, error) {
	if len(tracks) == 0 {
		return make([]int64, 0), nil
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
		collected := make([]int64, 0, len(tracks))
		for _, track := range tracks {
			maxPos += queuePositionGap
			res, err := tx.Exec(`
				INSERT INTO queue (track_hash, position, added_at, source_playlist, deleted_at)
				VALUES (?, ?, ?, 'Radio', ?)`,
				track.Hash, maxPos, now, pendingRadioDeletedAt,
			)
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			collected = append(collected, id)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		ids = collected
		return nil
	})
	return ids, err
}

// ActivateRadioTracks publishes an accepted pending batch atomically. Every
// ID must still carry the pending marker; otherwise no row is activated.
func (w *Writer) ActivateRadioTracks(ids []int64) error {
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
			result, err := tx.Exec(
				`UPDATE queue SET deleted_at=NULL WHERE id=? AND source_playlist='Radio' AND deleted_at=?`,
				id, pendingRadioDeletedAt,
			)
			if err != nil {
				return err
			}
			updated, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if updated != 1 {
				return fmt.Errorf("activate pending Radio queue row %d: updated %d rows", id, updated)
			}
		}
		return tx.Commit()
	})
}

// recoverPendingRadioTracks turns batches abandoned by a prior process into
// ordinary soft-deleted rows. NewWriter calls it before ReadQueue is used by
// startup, so the rows stay hidden and later become eligible for normal purge.
func (w *Writer) recoverPendingRadioTracks() {
	if _, err := w.db.Exec(
		`UPDATE queue SET deleted_at=? WHERE source_playlist='Radio' AND deleted_at=?`,
		time.Now().Unix(), pendingRadioDeletedAt,
	); err != nil {
		slog.Warn("db writer: recover pending Radio tracks", "err", err)
	}
}
