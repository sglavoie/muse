package db

import (
	"database/sql"
	"time"
)

// DismissTrack inserts a track dismissal. The operation is idempotent: if the
// hash is already dismissed the insert is ignored.
func (w *Writer) DismissTrack(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO recommendation_dismissals
			 (type, value, dismissed_at) VALUES ('track', ?, ?)`,
			hash, time.Now().Unix())
		return err
	})
}

// UndismissTrack removes a track dismissal. It is a no-op when no matching
// row exists.
func (w *Writer) UndismissTrack(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`DELETE FROM recommendation_dismissals
			 WHERE type = 'track' AND value = ?`, hash)
		return err
	})
}
