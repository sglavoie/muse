package db

import (
	"database/sql"
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
func (w *Writer) DeleteTagSoft(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tags SET deleted_at = ? WHERE id = ?`,
			time.Now().Unix(), id,
		)
		return err
	})
}

// UndoDeleteTag clears deleted_at, restoring a soft-deleted tag.
func (w *Writer) UndoDeleteTag(id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE tags SET deleted_at = NULL WHERE id = ?`, id)
		return err
	})
}

// PurgeDeletedTags hard-deletes tags whose deleted_at is older than before
// (Unix timestamp). Their track_tags rows are removed by ON DELETE CASCADE.
func (w *Writer) PurgeDeletedTags(before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM tags WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
			before,
		)
		return err
	})
}

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
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM track_tags tt
			JOIN tracks t ON tt.track_hash = t.hash
			WHERE tt.tag_id = ? AND t.missing = 0
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
func (w *Writer) MergeTags(sourceID, destID int64) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

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
