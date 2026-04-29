package db

import (
	"database/sql"
	"time"

	"github.com/sglavoie/muse/internal/recommendation"
)

// CacheEntry is a type alias for recommendation.CacheEntry, kept for backward
// compatibility so callers using db.CacheEntry continue to compile unchanged.
type CacheEntry = recommendation.CacheEntry

// RebuildRecommendationCache replaces the entire recommendation_cache table
// with the supplied entries inside a single transaction. The DELETE + INSERT
// is safe under WAL mode: concurrent readers see the pre-transaction snapshot
// until COMMIT.
func (w *Writer) RebuildRecommendationCache(entries []CacheEntry) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if _, err := tx.Exec(`DELETE FROM recommendation_cache`); err != nil {
			return err
		}

		if len(entries) == 0 {
			return tx.Commit()
		}

		now := time.Now().Unix()
		stmt, err := tx.Prepare(
			`INSERT INTO recommendation_cache (mix_id, track_hash, score, computed_at)
			 VALUES (?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, e := range entries {
			if _, err := stmt.Exec(e.MixID, e.TrackHash, e.Score, now); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}
