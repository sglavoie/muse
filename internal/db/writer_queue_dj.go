package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PrepareDJBatch persists a DJ batch as hidden rows at the end of the queue and
// returns the handle needed to accept or reject it. Nothing is visible to
// ReadQueue until AcceptDJBatch runs, so a crash between planning and
// acceptance cannot resurrect a batch the user never saw.
func (w *Writer) PrepareDJBatch(entries []DJQueueEntry) (DJPreparation, error) {
	return w.prepareDJ(DJPrepareAppend, entries, nil)
}

// PrepareDJReplacement persists a refresh: the same hidden rows, plus the exact
// set of active DJ-owned rows they retire when the batch is accepted.
//
// replacing must name DJ-owned rows only. Manual entries — explicit enqueues,
// pasted copies, playlist-derived copies, and every row that predates
// provenance — are refused with ErrQueueRowProtected here and again at accept,
// so applying new settings can never discard something the user queued.
func (w *Writer) PrepareDJReplacement(entries []DJQueueEntry, replacing []int64) (DJPreparation, error) {
	return w.prepareDJ(DJPrepareReplace, entries, replacing)
}

// prepareDJ is the shared preparation path.
func (w *Writer) prepareDJ(kind DJPreparationKind, entries []DJQueueEntry, replacing []int64) (DJPreparation, error) {
	if err := checkDJPreparation(kind, len(entries), len(replacing)); err != nil {
		return DJPreparation{}, err
	}

	rows, err := djQueueInserts(kind, entries)
	if err != nil {
		return DJPreparation{}, err
	}

	prep := DJPreparation{Kind: kind, Replacing: dedupeIDs(replacing)}
	err = w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		// Reject a protected target before writing anything, so a caller bug
		// does not leave hidden rows behind that only fail at accept.
		for _, id := range prep.Replacing {
			if _, err := classifyReplacementTarget(tx, id); err != nil {
				return err
			}
		}

		ids, err := appendQueueRows(tx, rows)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		prep.IDs = ids
		return nil
	})
	if err != nil {
		return DJPreparation{}, err
	}
	if prep.IDs == nil {
		prep.IDs = []int64{}
	}
	return prep, nil
}

// AcceptDJBatch publishes a prepared batch and commits the DJ state it advanced
// in one SQLite transaction: the retired rows, the newly visible rows, the
// active settings snapshot, and the allocation carry either all land or none
// do. A failure part-way through leaves the batch pending and re-acceptable.
//
// Retired rows are soft-deleted, not erased, so the ordinary undo window still
// applies to them and their provenance survives a restore.
func (w *Writer) AcceptDJBatch(prep DJPreparation, commit DJCommit) (DJAcceptance, error) {
	if prep.Kind == DJPrepareAppend && len(prep.Replacing) > 0 {
		return DJAcceptance{}, fmt.Errorf("%w: an append cannot replace rows", ErrQueuePreparationInvalid)
	}
	// Validate the DJ state before the transaction opens, matching
	// WriteDJSession: invalid settings must never reach the database, and
	// refusing here means the batch is still pending and can be retried.
	pending, err := prepareDJCommit(commit)
	if err != nil {
		return DJAcceptance{}, err
	}

	var accepted DJAcceptance
	err = w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		removed, err := applyDJBatch(tx, prep, pending)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		accepted = DJAcceptance{
			Activated:    append([]int64(nil), prep.IDs...),
			Removed:      removed,
			Replacements: pairReplacements(removed, prep.IDs),
		}
		return nil
	})
	if err != nil {
		return DJAcceptance{}, err
	}
	return accepted, nil
}

// applyDJBatch retires, publishes, and commits DJ state inside tx, returning
// the rows it actually retired. Every statement shares the transaction, so any
// error here leaves the batch pending and the queue exactly as it was.
func applyDJBatch(tx *sql.Tx, prep DJPreparation, commit djCommitWrite) ([]int64, error) {
	removed, err := retireReplacedRows(tx, prep.Replacing)
	if err != nil {
		return nil, err
	}
	for _, id := range prep.IDs {
		if err := activatePendingQueueRow(tx, id, `dj_owned=?`, 1); err != nil {
			return nil, err
		}
	}
	if err := commit.apply(tx); err != nil {
		return nil, err
	}
	return removed, nil
}

// checkDJPreparation rejects the shapes no preparation may take: an append that
// claims to retire rows, and a preparation that would neither add nor remove
// anything.
func checkDJPreparation(kind DJPreparationKind, entries, replacing int) error {
	if kind == DJPrepareAppend && replacing > 0 {
		return fmt.Errorf("%w: an append cannot replace rows", ErrQueuePreparationInvalid)
	}
	if entries == 0 && replacing == 0 {
		return fmt.Errorf("%w: nothing to prepare", ErrQueuePreparationInvalid)
	}
	return nil
}

// djQueueInserts renders planner selections as the hidden rows a preparation
// writes.
func djQueueInserts(kind DJPreparationKind, entries []DJQueueEntry) ([]queueInsert, error) {
	rows := make([]queueInsert, 0, len(entries))
	for _, entry := range entries {
		reasons, err := encodeQueueReasons(entry.Reasons)
		if err != nil {
			return nil, err
		}
		rows = append(rows, queueInsert{
			hash:    entry.Track.Hash,
			source:  radioSourceLabel,
			djOwned: 1,
			reasons: reasons,
			kind:    kind,
			hidden:  true,
		})
	}
	return rows, nil
}

// RejectDJBatch discards a prepared batch, turning its hidden rows into
// ordinary tombstones so the normal purge reclaims them. Rows that are no
// longer pending are skipped: rejecting a batch restart recovery already
// cleaned up is not an error.
func (w *Writer) RejectDJBatch(prep DJPreparation) error {
	if len(prep.IDs) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, id := range prep.IDs {
			if _, err := tx.Exec(
				`UPDATE queue SET deleted_at=?, pending_kind=? WHERE id=? AND deleted_at=?`,
				now, pendingKindNone, id, pendingQueueDeletedAt,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// retireReplacedRows soft-deletes the DJ-owned rows named by ids and returns
// the ones it actually retired, in the order given.
//
// A row that has already been dequeued is skipped rather than failing the
// accept: the user removing a track while a refresh was being planned should
// not cost them the refresh. A row that is active but not DJ-owned aborts the
// whole transaction.
func retireReplacedRows(tx *sql.Tx, ids []int64) ([]int64, error) {
	removed := make([]int64, 0, len(ids))
	now := time.Now().Unix()
	for _, id := range ids {
		present, err := classifyReplacementTarget(tx, id)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		if _, err := tx.Exec(`UPDATE queue SET deleted_at=? WHERE id=?`, now, id); err != nil {
			return nil, err
		}
		removed = append(removed, id)
	}
	return removed, nil
}

// classifyReplacementTarget reports whether id names an active row the DJ may
// retire. It returns false with no error when the row is absent or already
// soft-deleted, and ErrQueueRowProtected when the row exists but is not
// DJ-owned.
func classifyReplacementTarget(tx *sql.Tx, id int64) (bool, error) {
	var djOwned int
	err := tx.QueryRow(
		`SELECT dj_owned FROM queue WHERE id=? AND deleted_at IS NULL`, id,
	).Scan(&djOwned)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("db: look up queue row %d: %w", id, err)
	}
	if djOwned != 1 {
		return false, fmt.Errorf("%w: queue row %d", ErrQueueRowProtected, id)
	}
	return true, nil
}

// pairReplacements maps each retired row onto the row that took its place.
// Extra retirements pair with 0; extra activations are plain additions and
// appear only in DJAcceptance.Activated.
func pairReplacements(removed, activated []int64) []DJReplacement {
	if len(removed) == 0 {
		return nil
	}
	pairs := make([]DJReplacement, 0, len(removed))
	for i, oldID := range removed {
		var newID int64
		if i < len(activated) {
			newID = activated[i]
		}
		pairs = append(pairs, DJReplacement{OldID: oldID, NewID: newID})
	}
	return pairs
}

// dedupeIDs copies ids, dropping repeats and non-positive values so a caller's
// duplicate cannot retire the same row twice or pair it twice.
func dedupeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
