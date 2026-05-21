package db

import (
	"database/sql"
	"errors"
	"time"
)

// CreateSmartPlaylist inserts a new smart playlist with the given name and
// serialised JSON query. Returns the new row's ID.
func (w *Writer) CreateSmartPlaylist(name, queryJSON string) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(
			`INSERT INTO playlists (name, query, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			name, queryJSON, now, now,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// UpdateSmartPlaylistQuery replaces the name and query JSON for the smart
// playlist identified by id. It is a no-op if no row with that ID exists.
func (w *Writer) UpdateSmartPlaylistQuery(id int64, name, queryJSON string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE playlists SET name=?, query=?, updated_at=? WHERE id=?`,
			name, queryJSON, time.Now().Unix(), id,
		)
		return err
	})
}

// CreatePlaylist inserts a new playlist with the given name and returns its ID.
// The playlist is immediately active (deleted_at IS NULL).
func (w *Writer) CreatePlaylist(name string) (int64, error) {
	var id int64
	err := w.Exec(func(db *sql.DB) error {
		now := time.Now().Unix()
		res, err := db.Exec(
			`INSERT INTO playlists (name, created_at, updated_at) VALUES (?, ?, ?)`,
			name, now, now,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RenamePlaylist updates the name of the playlist identified by id.
// It is a no-op if no active playlist with that ID exists.
func (w *Writer) RenamePlaylist(id int64, name string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE playlists SET name=?, updated_at=? WHERE id=?`,
			name, time.Now().Unix(), id,
		)
		return err
	})
}

// DeletePlaylistSoft soft-deletes the playlist identified by id by setting its
// deleted_at timestamp. The row is retained for the undo window.
func (w *Writer) DeletePlaylistSoft(id int64) error { return w.softDeleteByID("playlists", id) }

// UndoDeletePlaylist clears the deleted_at flag on the playlist identified by
// id, restoring it to the active set. Call this within the undo window to
// reverse a DeletePlaylistSoft.
func (w *Writer) UndoDeletePlaylist(id int64) error {
	return w.undoSoftDeleteByID("playlists", id)
}

// PurgeDeletedPlaylists physically removes all playlist rows whose deleted_at
// is non-NULL and strictly less than the before Unix timestamp.
// Call this once the undo window has expired to reclaim space.
func (w *Writer) PurgeDeletedPlaylists(before int64) error {
	return w.purgeSoftDeleted("playlists", before)
}

// AddTracksToPlaylist appends hashes to playlistID using gapped-integer
// positioning (MAX(position)+1000 per track). All inserts run inside a single
// transaction. It is a no-op when hashes is empty.
func (w *Writer) AddTracksToPlaylist(playlistID int64, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var maxPos int64
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), 0) FROM playlist_tracks WHERE playlist_id=?`,
			playlistID,
		).Scan(&maxPos); err != nil {
			return err
		}

		now := time.Now().Unix()
		for _, hash := range hashes {
			maxPos += queuePositionGap
			if _, err := tx.Exec(
				`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, ?, ?, ?)`,
				playlistID, hash, maxPos, now,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// RemovePlaylistTrackSoft soft-deletes the playlist_tracks row identified by
// id by setting its deleted_at timestamp. The row is retained for the undo
// window and can be restored via UndoRemovePlaylistTrack.
func (w *Writer) RemovePlaylistTrackSoft(id int64) error {
	return w.softDeleteByID("playlist_tracks", id)
}

// UndoRemovePlaylistTrack clears the deleted_at flag on the playlist_tracks
// row identified by id, restoring it to the active set. Call this within the
// undo window to reverse a RemovePlaylistTrackSoft.
func (w *Writer) UndoRemovePlaylistTrack(id int64) error {
	return w.undoSoftDeleteByID("playlist_tracks", id)
}

// ReorderPlaylistTrack performs a single-step adjacent swap for the track
// identified by trackID inside playlistID. direction must be -1 (move up,
// toward lower position) or +1 (move down, toward higher position). It is a
// no-op when the track is already at the boundary. Only active
// (deleted_at IS NULL) neighbours are considered.
func (w *Writer) ReorderPlaylistTrack(playlistID, trackID int64, direction int) error {
	if direction != -1 && direction != 1 {
		return errors.New("direction must be -1 or +1")
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		var currPos int64
		if err := tx.QueryRow(
			`SELECT position FROM playlist_tracks WHERE id=? AND playlist_id=? AND deleted_at IS NULL`,
			trackID, playlistID,
		).Scan(&currPos); err != nil {
			return err
		}

		var adjQuery string
		if direction > 0 {
			adjQuery = `SELECT id, position FROM playlist_tracks
				WHERE playlist_id=? AND deleted_at IS NULL AND position > ?
				ORDER BY position ASC LIMIT 1`
		} else {
			adjQuery = `SELECT id, position FROM playlist_tracks
				WHERE playlist_id=? AND deleted_at IS NULL AND position < ?
				ORDER BY position DESC LIMIT 1`
		}

		var adjID, adjPos int64
		if err := tx.QueryRow(adjQuery, playlistID, currPos).Scan(&adjID, &adjPos); err != nil {
			if err == sql.ErrNoRows {
				return nil // already at boundary
			}
			return err
		}

		// Three-step swap to avoid transient unique-constraint violations.
		// Negative positions cannot exist in normal use so -1 is a safe temp.
		const tempPos int64 = -1
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, tempPos, trackID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, currPos, adjID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE playlist_tracks SET position=? WHERE id=?`, adjPos, trackID); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// SetPlaylistOrder reorders the tracks of a playlist by assigning new gapped
// positions in the order given by ids (which are playlist_tracks.id values).
// Soft-deleted rows that would collide with the new positions are relocated
// first. This is the playlist equivalent of SetQueueOrder.
func (w *Writer) SetPlaylistOrder(playlistID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		if err := setGappedOrderTx(tx, "playlist_tracks", "AND playlist_id=?", []any{playlistID}, ids); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// PurgeDeletedPlaylistTracks physically removes all playlist_tracks rows whose
// deleted_at is non-NULL and strictly less than the before Unix timestamp.
// Call this once the undo window has expired to reclaim space.
func (w *Writer) PurgeDeletedPlaylistTracks(before int64) error {
	return w.purgeSoftDeleted("playlist_tracks", before)
}
