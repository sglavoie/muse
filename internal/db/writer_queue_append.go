package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// pendingQueueDeletedAt is a durable marker for a batch whose scoring command
// committed but whose result has not yet been accepted by the TUI. It is
// deliberately later than any real Unix timestamp, so ordinary soft-delete
// purges cannot remove an in-flight batch before it is activated, and the undo
// and dequeue paths can recognise a row that is not a tombstone.
const pendingQueueDeletedAt int64 = 1<<63 - 1

// AppendTrack inserts track at the end of the play queue using a gapped
// position (MAX(position)+1000) and returns the new row's ID. The track must
// already exist in the tracks table (i.e., it was previously scanned).
// Duplicate queue entries for the same track hash are accepted without error.
// sourcePlaylist identifies the playlist the track was enqueued from; pass ""
// for tracks enqueued individually (e.g. from the library view).
//
// Rows written here are manual entries: this is the boundary explicit enqueues,
// pasted copies, and playlist-derived copies come through, and a copy of a DJ
// selection belongs to the user, not to the DJ. Use RestoreQueueTrack when undo
// has to recreate a row whose provenance must survive.
func (w *Writer) AppendTrack(track library.Track, sourcePlaylist string) (int64, error) {
	return w.appendTrack(track, sourcePlaylist, ManualProvenance())
}

// RestoreQueueTrack re-appends a queue entry, preserving the provenance the row
// carried before it was removed. Undo paths that cannot restore a row in place —
// because the original was already replaced, or because the entry has to move —
// must use this rather than AppendTrack, which marks everything it writes as a
// manual entry.
func (w *Writer) RestoreQueueTrack(track library.Track, sourcePlaylist string, provenance QueueProvenance) (int64, error) {
	return w.appendTrack(track, sourcePlaylist, provenance)
}

// appendTrack is the shared single-row append.
func (w *Writer) appendTrack(track library.Track, sourcePlaylist string, provenance QueueProvenance) (int64, error) {
	reasons, err := encodeQueueReasons(provenance.Reasons)
	if err != nil {
		return 0, err
	}
	var id int64
	err = w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(fmt.Sprintf(`
			INSERT INTO queue (track_hash, position, added_at, source_playlist, dj_owned, dj_reasons)
			VALUES (?, COALESCE((SELECT MAX(position) FROM queue), 0) + %d, ?, ?, ?, ?)`, queuePositionGap),
			track.Hash, now, sourcePlaylist, provenance.djOwnedFlag(), reasons,
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
//
// Like AppendTrack, every row is a manual entry. Enqueueing a whole playlist
// named "Radio" therefore produces protected rows, not DJ-owned ones.
func (w *Writer) AppendTracks(tracks []library.Track, sourcePlaylist string) ([]int64, error) {
	if len(tracks) == 0 {
		return make([]int64, 0), nil
	}
	rows := make([]queueInsert, 0, len(tracks))
	for _, t := range tracks {
		rows = append(rows, queueInsert{hash: t.Hash, source: sourcePlaylist})
	}
	var ids []int64
	err := w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		collected, err := appendQueueRows(tx, rows)
		if err != nil {
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

// queueInsert describes one row to append at the tail of the queue.
//
// hidden rows carry pendingQueueDeletedAt in deleted_at plus a pending_kind, so
// ReadQueue cannot see them until they are accepted.
type queueInsert struct {
	hash    string
	source  string
	djOwned int
	reasons string
	kind    DJPreparationKind
	hidden  bool
}

// appendQueueRows inserts rows at the tail of the queue in order, each one gap
// above the previous, and returns their ids in the same order.
//
// New rows always go to the tail even when they replace rows in the middle of
// the queue: queue_pos_idx is unique across soft-deleted rows too, so a
// displaced row's position is not reusable until it is purged.
func appendQueueRows(tx *sql.Tx, rows []queueInsert) ([]int64, error) {
	var maxPos int64
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), 0) FROM queue`,
	).Scan(&maxPos); err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		maxPos += queuePositionGap
		var deletedAt any
		if row.hidden {
			deletedAt = pendingQueueDeletedAt
		}
		res, err := tx.Exec(`
			INSERT INTO queue (track_hash, position, added_at, source_playlist, deleted_at, dj_owned, dj_reasons, pending_kind)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			row.hash, maxPos, now, row.source, deletedAt, row.djOwned, row.reasons, string(row.kind),
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// AppendRadioTracksPending persists a Radio batch without making it visible to
// ReadQueue. The TUI activates the exact IDs only after confirming the async
// result still belongs to the current Radio request. A crash before that point
// therefore cannot restore a discarded batch on the next launch.
//
// The rows are labelled "Radio" for display but are NOT marked DJ-owned: this
// is the pre-DJ radio path, and ownership is only ever claimed by the steerable
// DJ through PrepareDJBatch. See PrepareDJBatch for the replacement-capable
// successor.
func (w *Writer) AppendRadioTracksPending(tracks []library.Track) ([]int64, error) {
	if len(tracks) == 0 {
		return make([]int64, 0), nil
	}
	rows := make([]queueInsert, 0, len(tracks))
	for _, track := range tracks {
		rows = append(rows, queueInsert{
			hash:   track.Hash,
			source: radioSourceLabel,
			kind:   DJPrepareAppend,
			hidden: true,
		})
	}

	var ids []int64
	err := w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		collected, err := appendQueueRows(tx, rows)
		if err != nil {
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
			if err := activatePendingQueueRow(tx, id, `source_playlist=?`, radioSourceLabel); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// activatePendingQueueRow publishes one hidden row. extraWhere and its argument
// narrow the match so a caller can only activate rows it actually prepared; the
// pending marker itself is always required, so a row that was recovered,
// rejected, or already published can never be resurrected.
func activatePendingQueueRow(tx *sql.Tx, id int64, extraWhere string, extraArg any) error {
	result, err := tx.Exec(
		`UPDATE queue SET deleted_at=NULL, pending_kind='' WHERE id=? AND deleted_at=? AND `+extraWhere, //nolint:gosec // extraWhere is a hardcoded internal fragment, not user input
		id, pendingQueueDeletedAt, extraArg,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("%w: queue row %d: updated %d rows", ErrQueueBatchStale, id, updated)
	}
	return nil
}

// recoverPendingQueueEntries turns preparations abandoned by a prior process
// into ordinary soft-deleted rows. NewWriter calls it before ReadQueue is used
// by startup, so the rows stay hidden and later become eligible for normal
// purge.
//
// It keys on the pending marker alone rather than on a source label, so it
// covers legacy Radio batches and DJ preparations alike, and cannot touch a row
// that merely came from a playlist the user named "Radio".
func (w *Writer) recoverPendingQueueEntries() {
	if _, err := w.db.Exec(
		`UPDATE queue SET deleted_at=?, pending_kind='' WHERE deleted_at=?`,
		time.Now().Unix(), pendingQueueDeletedAt,
	); err != nil {
		slog.Warn("db writer: recover pending queue entries", "err", err)
	}
}
