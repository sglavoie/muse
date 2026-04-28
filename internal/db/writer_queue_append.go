package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

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
