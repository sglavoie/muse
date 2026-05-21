package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateTag inserts a new active tag with the given name and color. Returns the
// new tag ID. Returns an error when a tag with the same name already exists
// (COLLATE NOCASE uniqueness is enforced by the schema).
func (w *Writer) CreateTag(name, color string) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		res, err := db.Exec(
			`INSERT INTO tags (name, color, created_at) VALUES (?, ?, ?)`,
			name, color, time.Now().Unix(),
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RenameTag updates the name of the tag with the given ID.
func (w *Writer) RenameTag(id int64, name string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE tags SET name = ? WHERE id = ?`, name, id)
		return err
	})
}

// UpdateTagColor updates the color of the tag with the given ID.
func (w *Writer) UpdateTagColor(id int64, color string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE tags SET color = ? WHERE id = ?`, color, id)
		return err
	})
}

// DeleteTagSoft sets deleted_at on the tag, hiding it from active queries.
// The tag and its track_tags rows remain in the database for undo support.
func (w *Writer) DeleteTagSoft(id int64) error { return w.softDeleteByID("tags", id) }

// UndoDeleteTag clears deleted_at, restoring a soft-deleted tag.
func (w *Writer) UndoDeleteTag(id int64) error { return w.undoSoftDeleteByID("tags", id) }

// PurgeDeletedTags hard-deletes tags whose deleted_at is older than before
// (Unix timestamp). Their track_tags rows are removed by ON DELETE CASCADE.
func (w *Writer) PurgeDeletedTags(before int64) error { return w.purgeSoftDeleted("tags", before) }

// AddTrackTag assigns a tag to a track. The operation is idempotent: if the
// (track_hash, tag_id) pair already exists the insert is ignored.
func (w *Writer) AddTrackTag(hash string, tagID int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO track_tags (track_hash, tag_id, created_at) VALUES (?, ?, ?)`,
			hash, tagID, time.Now().Unix(),
		)
		return err
	})
}

// RemoveAllTrackTags removes every tag assignment for the given track hash
// inside a single transaction. It is a no-op when the track has no tags.
func (w *Writer) RemoveAllTrackTags(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`DELETE FROM track_tags WHERE track_hash = ?`, hash)
		return err
	})
}

// RemoveTrackTag removes the assignment of a tag from a track. It is a no-op
// when no matching row exists.
func (w *Writer) RemoveTrackTag(hash string, tagID int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM track_tags WHERE track_hash = ? AND tag_id = ?`,
			hash, tagID,
		)
		return err
	})
}

// AddTrackTagBulk assigns a tag to multiple tracks inside a single
// transaction. Each insert is idempotent (INSERT OR IGNORE). It is a no-op
// when hashes is empty.
func (w *Writer) AddTrackTagBulk(hashes []string, tagID int64) error {
	if len(hashes) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, hash := range hashes {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO track_tags (track_hash, tag_id, created_at) VALUES (?, ?, ?)`,
				hash, tagID, now,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// RemoveTrackTagBulk removes the assignment of a tag from multiple tracks
// inside a single transaction. It is a no-op when hashes is empty.
func (w *Writer) RemoveTrackTagBulk(hashes []string, tagID int64) error {
	if len(hashes) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		for _, hash := range hashes {
			if _, err := tx.Exec(
				`DELETE FROM track_tags WHERE track_hash = ? AND tag_id = ?`,
				hash, tagID,
			); err != nil {
				return err
			}
		}

		return tx.Commit()
	})
}

// DeleteTagIfEmpty soft-deletes the tag when it has no remaining non-missing
// tracks. Returns true when the tag was deleted, false when tracks still exist.
// It is safe to call for a tag that still has tracks (no-op).
func (w *Writer) DeleteTagIfEmpty(tagID int64) (bool, error) {
	var deleted bool
	err := w.Exec(func(db *sql.DB) error {
		// Count every track_tags row carrying this tag, including ones whose
		// referenced track is currently flagged missing. Otherwise the tag
		// could be silently soft-deleted while assignments still exist on
		// missing tracks, orphaning those assignments to a deleted tag (they
		// become invisible to ReadTagsForTrack which filters deleted_at IS
		// NULL) and silently destroying user data when the file is restored.
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM track_tags tt
			WHERE tt.tag_id = ?
		`, tagID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		res, err := db.Exec(
			`UPDATE tags SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
			time.Now().Unix(), tagID,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		deleted = n > 0
		return nil
	})
	return deleted, err
}

// MergeTags re-points all track_tags rows from sourceID to destID and
// soft-deletes the source tag, all within a single transaction. Rows that
// would create a duplicate (track_hash, destID) pair are silently ignored via
// INSERT OR IGNORE. This operation is not reversible.
//
// A guard rejects self-merge (sourceID == destID) because the natural sequence
// (INSERT OR IGNORE same rows → DELETE all source rows → soft-delete source)
// would otherwise wipe every track_tags row for the tag and then soft-delete
// the tag itself, silently destroying user data.
//
// A second guard rejects merging into a destination tag that does not exist
// or is already soft-deleted. Without this guard, every re-pointed track_tags
// row would land on a tag that ReadTagsForTrack hides (it filters
// deleted_at IS NULL), silently making the user's tag assignments invisible
// while the source tag is also soft-deleted in the same transaction.
func (w *Writer) MergeTags(sourceID, destID int64) error {
	if sourceID == destID {
		return fmt.Errorf("merge tags: source and destination IDs must differ (got %d)", sourceID)
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		// Verify the source tag exists and is active. Without this guard a
		// caller passing a missing or soft-deleted source ID would receive a
		// nil error even though zero rows were re-pointed, masking the bug
		// and giving the impression the merge succeeded.
		var srcDeletedAt sql.NullInt64
		if err := tx.QueryRow(
			`SELECT deleted_at FROM tags WHERE id = ?`, sourceID,
		).Scan(&srcDeletedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("merge tags: source tag %d does not exist", sourceID)
			}
			return fmt.Errorf("merge tags lookup source: %w", err)
		}
		if srcDeletedAt.Valid {
			return fmt.Errorf("merge tags: source tag %d is soft-deleted; restore it before merging", sourceID)
		}

		// Verify the destination tag exists and is active. Without this guard
		// re-pointed rows would attach to a soft-deleted tag invisible to
		// ReadTagsForTrack, destroying the user's tag data.
		var destDeletedAt sql.NullInt64
		if err := tx.QueryRow(
			`SELECT deleted_at FROM tags WHERE id = ?`, destID,
		).Scan(&destDeletedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("merge tags: destination tag %d does not exist", destID)
			}
			return fmt.Errorf("merge tags lookup destination: %w", err)
		}
		if destDeletedAt.Valid {
			return fmt.Errorf("merge tags: destination tag %d is soft-deleted; restore it before merging", destID)
		}

		// Re-point: insert rows pointing at destID for every track currently
		// under sourceID, ignoring any that already exist under destID.
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO track_tags (track_hash, tag_id, created_at)
			SELECT track_hash, ?, created_at FROM track_tags WHERE tag_id = ?
		`, destID, sourceID); err != nil {
			return fmt.Errorf("merge tags re-point: %w", err)
		}

		// Remove the now-redundant source rows.
		if _, err := tx.Exec(
			`DELETE FROM track_tags WHERE tag_id = ?`, sourceID,
		); err != nil {
			return fmt.Errorf("merge tags delete source rows: %w", err)
		}

		// Soft-delete the source tag.
		if _, err := tx.Exec(
			`UPDATE tags SET deleted_at = ? WHERE id = ?`,
			time.Now().Unix(), sourceID,
		); err != nil {
			return fmt.Errorf("merge tags soft-delete source: %w", err)
		}

		return tx.Commit()
	})
}
