package db

import (
	"database/sql"
	"time"
)

// softDeleteByID sets deleted_at to the current Unix time on the row in table
// identified by id. table must be a hardcoded internal name.
func (w *Writer) softDeleteByID(table string, id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE `+table+` SET deleted_at=? WHERE id=?`, //nolint:gosec // table is a hardcoded internal name, not user input
			time.Now().Unix(), id,
		)
		return err
	})
}

// undoSoftDeleteByID clears deleted_at on the row in table identified by id,
// restoring it to the active set. table must be a hardcoded internal name.
func (w *Writer) undoSoftDeleteByID(table string, id int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE `+table+` SET deleted_at=NULL WHERE id=?`, //nolint:gosec // table is a hardcoded internal name, not user input
			id,
		)
		return err
	})
}

// purgeSoftDeleted hard-deletes all rows in table whose deleted_at is non-NULL
// and strictly less than the before Unix timestamp.
// table must be a hardcoded internal name.
func (w *Writer) purgeSoftDeleted(table string, before int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM `+table+` WHERE deleted_at IS NOT NULL AND deleted_at < ?`, //nolint:gosec // table is a hardcoded internal name, not user input
			before,
		)
		return err
	})
}
