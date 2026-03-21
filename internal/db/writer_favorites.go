package db

import (
	"database/sql"
	"time"
)

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
